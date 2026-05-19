import os
import sys
import json
import logging
import argparse
from datetime import datetime

import pandas as pd
import numpy as np
import joblib
import psycopg2
from sklearn.ensemble import IsolationForest
from sklearn.preprocessing import StandardScaler

from config import *


logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('train_model.log'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)






def load_data_from_db(days):

    logger.info("Подключение к БД: %s:%s/%s", DB_HOST, DB_PORT, DB_NAME)

    try:
        conn = psycopg2.connect(
            host=DB_HOST,
            port=DB_PORT,
            dbname=DB_NAME,
            user=DB_USER,
            password=DB_PASSWORD,
            connect_timeout=10
        )
        logger.info("Подключены успешно")
    except Exception as e:
        logger.error("Ошибка подключения к БД: %s", e)
        return None

    query = """
        SELECT 
            cpu_usage as cpu,
            ram_usage as ram,
            disk_usage as disk,
            cpu_temp as temperature,
            core_diff,
            pps_in + pps_out as pps,
            bps_in + bps_out as bps
        FROM metrics
        WHERE timestamp > NOW() - INTERVAL '%s days'
    """ % days

    try:
        logger.info("Загрузка данных за %s дней...", days)
        df = pd.read_sql(query, conn)

        rows_before = len(df)
        df = df.dropna()
        rows_after = len(df)
        dropped = rows_before - rows_after

        logger.info("Загружено строк: %d (удалено NaN: %d)", rows_after, dropped)

        return df

    except Exception as e:
        logger.error("Ошибка загрузки данных: %s", e)
        return None
    finally:
        conn.close()


def auto_detect_contamination(df):

    outlier_ratios = []

    for col in ['pps', 'bps', 'cpu']:
        if col not in df.columns:
            continue

        q1 = df[col].quantile(0.25)
        q3 = df[col].quantile(0.75)
        iqr = q3 - q1
        upper = q3 + 1.5 * iqr


        ratio = (df[col] > upper).mean()
        outlier_ratios.append(ratio)

    if not outlier_ratios:
        return 0.05

    contamination = float(np.mean(outlier_ratios)) * 1.5


    contamination = max(0.01, min(0.10, contamination))

    logger.info("Автоопределение contamination:")
    logger.info("  Выбросы по признакам: %s",
                [round(r, 4) for r in outlier_ratios])
    logger.info("  Итоговый contamination: %.4f (%.2f%%)",
                contamination, contamination * 100)

    return contamination


def evaluate_model(model, scaler, X):

    X_scaled = scaler.transform(X)
    predictions = model.predict(X_scaled)
    scores = model.decision_function(X_scaled)

    anomaly_count = (predictions == -1).sum()
    total = len(predictions)
    anomaly_ratio = anomaly_count / total

    logger.info("Оценка модели:")
    logger.info("  Всего образцов: %d", total)
    logger.info("  Аномалий: %d (%.2f%%)", anomaly_count, anomaly_ratio * 100)
    logger.info("  Anomaly score: min=%.3f, max=%.3f, mean=%.3f, median=%.3f",
                scores.min(), scores.max(), scores.mean(), np.median(scores))

    return {
        'total_samples': int(total),
        'anomaly_count': int(anomaly_count),
        'anomaly_ratio': float(anomaly_ratio),
        'score_min': float(scores.min()),
        'score_max': float(scores.max()),
        'score_mean': float(scores.mean()),
        'score_median': float(np.median(scores))
    }


def validate_on_test_set(df, contamination):


    split_idx = int(len(df) * 0.8)
    df_train = df.iloc[:split_idx]
    df_test = df.iloc[split_idx:]

    if len(df_test) < 100:
        logger.warning("Недостаточно данных для валидации: %d < 100", len(df_test))
        return None

    try:

        scaler_val = StandardScaler()
        X_train_scaled = scaler_val.fit_transform(df_train)

        model_val = IsolationForest(
            n_estimators=200,
            contamination=contamination,
            random_state=42,
            n_jobs=-1,
            bootstrap=True
        )
        model_val.fit(X_train_scaled)


        X_test_scaled = scaler_val.transform(df_test)
        preds = model_val.predict(X_test_scaled)

        test_anomaly_ratio = (preds == -1).mean()

        logger.info("Валидация на test-set (%d образцов):", len(df_test))
        logger.info("  Аномалий: %.2f%% (ожидалось ~%.2f%%)",
                    test_anomaly_ratio * 100, contamination * 100)


        if contamination > 0 and test_anomaly_ratio > contamination * 3:
            logger.warning("расхождение > 3x! Модель может быть нестабильна")
            stable = False
        elif contamination > 0 and test_anomaly_ratio < contamination * 0.1:
            logger.warning("аномалий слишком мало! Возможно contamination завышен")
            stable = False
        else:
            logger.info("Модель стабильна")
            stable = True

        return {
            'test_samples': len(df_test),
            'test_anomaly_ratio': float(test_anomaly_ratio),
            'expected_contamination': float(contamination),
            'stable': stable
        }

    except Exception as e:
        logger.error("Ошибка валидации: %s", e)
        return None


def save_model(model, scaler, metadata):

    try:

        os.makedirs(os.path.dirname(MODEL_PATH), exist_ok=True)


        joblib.dump(model, MODEL_PATH)
        joblib.dump(scaler, SCALER_PATH)


        meta_path = MODEL_PATH.replace('.pkl', '_meta.json')
        with open(meta_path, 'w', encoding='utf-8') as f:
            json.dump(metadata, f, indent=2, default=str, ensure_ascii=False)

        logger.info("Модель сохранена: %s", MODEL_PATH)
        logger.info("Скейлер сохранён: %s", SCALER_PATH)
        logger.info("Метаданные сохранены: %s", meta_path)

        return True

    except Exception as e:
        logger.error("Ошибка сохранения модели: %s", e)
        return False






def train_model(days=14, contamination=None, dry_run=False):

    logger.info("=" * 60)
    logger.info("ОБУЧЕНИЕ МОДЕЛИ")
    logger.info("Время начала: %s", datetime.now())
    logger.info("Период данных: %d дней", days)
    logger.info("Contamination: %s",
                '%.4f (явно)' % contamination if contamination else 'авто')
    logger.info("Dry run: %s", 'ДА (без сохранения)' if dry_run else 'НЕТ')
    logger.info("=" * 60)


    df = load_data_from_db(days)

    if df is None:
        logger.error("Не удалось загрузить данные")
        return False


    if len(df) < MIN_TRAINING_SAMPLES:
        logger.warning(
            "Недостаточно данных для обучения: %d < %d",
            len(df), MIN_TRAINING_SAMPLES
        )
        logger.warning(
            "Соберите больше метрик или уменьшите MIN_TRAINING_SAMPLES в .env"
        )
        return False


    if contamination is None:
        contamination = auto_detect_contamination(df)


    logger.info("Обучение IsolationForest...")

    try:

        scaler = StandardScaler()
        X_scaled = scaler.fit_transform(df)


        model = IsolationForest(
            n_estimators=200,
            contamination=contamination,
            random_state=42,
            n_jobs=-1,
            bootstrap=True
        )
        model.fit(X_scaled)

        logger.info("Модель обучена успешно")

    except Exception as e:
        logger.error("Ошибка обучения модели: %s", e, exc_info=True)
        return False

    stats = evaluate_model(model, scaler, df)

    validation = validate_on_test_set(df, contamination)

    if dry_run:
        logger.info("Dry run — модель НЕ сохранена")
        return True


    metadata = {
        'trained_at': datetime.utcnow().isoformat(),
        'days_of_data': days,
        'total_samples': stats['total_samples'],
        'contamination': contamination,
        'features': ML_FEATURES,
        'stats': stats,
        'validation': validation
    }

    success = save_model(model, scaler, metadata)

    if success:
        logger.info("=" * 60)
        logger.info("ОБУЧЕНИЕ ЗАВЕРШЕНО УСПЕШНО")
        logger.info("Модель готова к использованию")
        logger.info("=" * 60)
    else:
        logger.error("=" * 60)
        logger.error("ОБУЧЕНИЕ ЗАВЕРШЕНО С ОШИБКОЙ СОХРАНЕНИЯ")
        logger.error("=" * 60)

    return success



if __name__ == "__main__":

    parser = argparse.ArgumentParser(
        description='Обучение ML-модели для Detection Service'
    )
    parser.add_argument(
        '--days',
        type=int,
        default=TRAINING_DAYS,
        help='Количество дней истории для обучения (по умолчанию: %d)' % TRAINING_DAYS
    )
    parser.add_argument(
        '--contamination',
        type=float,
        default=None,
        help='Доля аномалий от 0.01 до 0.10 (по умолчанию: автоопределение)'
    )
    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Проверочный запуск без сохранения модели'
    )
    parser.add_argument(
        '--output',
        type=str,
        default=None,
        help='Альтернативный путь для сохранения модели'
    )

    args = parser.parse_args()


    if args.output:
        MODEL_PATH = args.output
        SCALER_PATH = args.output.replace('.pkl', '_scaler.pkl')
        os.environ['MODEL_PATH'] = MODEL_PATH
        os.environ['SCALER_PATH'] = SCALER_PATH


    success = train_model(
        days=args.days,
        contamination=args.contamination,
        dry_run=args.dry_run
    )

    sys.exit(0 if success else 1)