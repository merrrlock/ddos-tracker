


import os  #работа с файловой системой (проверка наличия модели)
import time  #для sleep в главном цикле и антиспам-кулдауна
import logging  #логирование в файл и stdout
from typing import Dict, List, Tuple  #аннотации типов для читаемости
from datetime import datetime  #метки времени в алертах
from collections import defaultdict  #словарь с автоматическим созданием ключей

import numpy as np  #работа с массивами для ML и статистики
import joblib  #загрузка/сохранение ML-модели и скейлера
import psycopg2  #подключение к PostgreSQL
from psycopg2.extras import RealDictCursor  # курсор, возвращающий словари (не кортежи)
import requests  #HTTP-запросы к API Леонида для отправки алертов

from config import *  #импорт всех настроек из config.py



logging.basicConfig(
    level=getattr(logging, LOG_LEVEL),
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler('/var/log/detector.log'),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)


class DetectionService:


    def __init__(self):


        self.model = None
        self.scaler = None
        self.use_ml = False


        self.db_conn = None




        self.last_alerts = defaultdict(lambda: defaultdict(float))
        self.alert_cooldown = ALERT_COOLDOWN




        self.metrics_history = defaultdict(list)
        self.history_max_len = 1440


        self._load_model()

        self._connect_db()


        logger.info("Сервис запущен. ML: %s, Статистические методы: ON",
                    'ON' if self.use_ml else 'OFF')



    def _connect_db(self):

        try:
            self.db_conn = psycopg2.connect(
                host=DB_HOST,
                port=DB_PORT,
                dbname=DB_NAME,
                user=DB_USER,
                password=DB_PASSWORD,
                connect_timeout=5
            )
            self.db_conn.autocommit = True
            logger.info("Подключены к БД")
        except Exception as e:
            logger.error("Ошибка подключения к БД: %s", e)
            self.db_conn = None

    def _ensure_db(self):


        if self.db_conn is None:
            self._connect_db()
            return

        try:

            with self.db_conn.cursor() as cur:
                cur.execute("SELECT 1")
        except Exception:

            logger.warning("Потеря соединения с БД, переподключение...")
            try:
                if self.db_conn:
                    self.db_conn.close()
            except Exception:
                pass
            self.db_conn = None
            self._connect_db()




    def _load_model(self):

        try:

            if os.path.exists(MODEL_PATH) and os.path.exists(SCALER_PATH):

                self.model = joblib.load(MODEL_PATH)

                self.scaler = joblib.load(SCALER_PATH)
                self.use_ml = True
                logger.info("ML модель загружена")
            else:

                logger.warning("Модель не найдена — работаем по правилам + z-score/IQR")
        except Exception as e:

            logger.error("Ошибка загрузки модели: %s", e)
            self.use_ml = False





    def get_latest_metrics(self):


        self._ensure_db()

        if not self.db_conn:
            return []

        try:
            with self.db_conn.cursor(cursor_factory=RealDictCursor) as cur:

                cur.execute("""
                            SELECT DISTINCT
                            ON (server_id)
                                server_id,
                                cpu_usage as cpu, 
                                ram_usage as ram, 
                                disk_usage as disk, 
                                cpu_temp as temperature,
                                core_diff, 
                                pps_in + pps_out as pps, 
                                bps_in + bps_out as bps, 
                                timestamp 
                            FROM metrics
                            WHERE timestamp > NOW() - INTERVAL '15 seconds'
                            ORDER BY server_id, timestamp DESC
                            """)
                metrics = cur.fetchall()


                cur.execute("""
                            SELECT s.id             as server_id, 
                                   s.name,                       
                                   MAX(m.timestamp) as last_seen  
                            FROM servers s
                                     LEFT JOIN metrics m ON s.id = m.server_id
                            GROUP BY s.id, s.name
                            HAVING MAX(m.timestamp) < NOW() - INTERVAL '60 seconds'
                                OR MAX (m.timestamp) IS NULL
                            """)
                offline_servers = cur.fetchall()


                for srv in offline_servers:
                    metrics.append({
                        'server_id': srv['server_id'],
                        'cpu': 0,
                        'ram': 0,
                        'disk': 0,
                        'temperature': 0,
                        'core_diff': 0,
                        'pps': 0,
                        'bps': 0,
                        'timestamp': srv['last_seen'],
                        '_offline': True
                    })

                return metrics

        except Exception as e:
            logger.error("Ошибка получения метрик: %s", e)
            self._connect_db()
            return []

    def get_historical_metrics(self, server_id, minutes=60):

        self._ensure_db()
        if not self.db_conn:
            return []

        try:
            with self.db_conn.cursor(cursor_factory=RealDictCursor) as cur:

                cur.execute("""
                            SELECT cpu_usage        as cpu,
                                   ram_usage        as ram,
                                   disk_usage       as disk,
                                   cpu_temp         as temperature,
                                   core_diff,
                                   pps_in + pps_out as pps,
                                   bps_in + bps_out as bps
                            FROM metrics
                            WHERE server_id = %s
                              AND timestamp
                                > NOW() - INTERVAL '%s minutes'
                            ORDER BY timestamp ASC 
                            """, (server_id, minutes))
                return cur.fetchall()
        except Exception as e:
            logger.error("Ошибка загрузки истории для %s: %s", server_id, e)
            return []




    def check_rules(self, m):

        alerts = []


        if m['cpu'] > THRESHOLDS['cpu_critical']:
            alerts.append({
                'type': 'cpu',
                'severity': 'critical',
                'message': "CPU %.1f%% (порог %.0f%%)" % (m['cpu'], THRESHOLDS['cpu_critical'])
            })
        elif m['cpu'] > THRESHOLDS['cpu_warning']:
            alerts.append({
                'type': 'cpu',
                'severity': 'warning',
                'message': "CPU %.1f%% (порог %.0f%%)" % (m['cpu'], THRESHOLDS['cpu_warning'])
            })


        if m['ram'] > THRESHOLDS['ram_critical']:
            alerts.append({
                'type': 'ram',
                'severity': 'critical',
                'message': "RAM %.1f%% (порог %.0f%%)" % (m['ram'], THRESHOLDS['ram_critical'])
            })
        elif m['ram'] > THRESHOLDS['ram_warning']:
            alerts.append({
                'type': 'ram',
                'severity': 'warning',
                'message': "RAM %.1f%% (порог %.0f%%)" % (m['ram'], THRESHOLDS['ram_warning'])
            })


        if m['disk'] > THRESHOLDS['disk_critical']:
            alerts.append({
                'type': 'disk',
                'severity': 'critical',
                'message': "Disk %.1f%% (порог %.0f%%)" % (m['disk'], THRESHOLDS['disk_critical'])
            })
        elif m['disk'] > THRESHOLDS['disk_warning']:
            alerts.append({
                'type': 'disk',
                'severity': 'warning',
                'message': "Disk %.1f%% (порог %.0f%%)" % (m['disk'], THRESHOLDS['disk_warning'])
            })


        if m['temperature'] > THRESHOLDS['temp_critical']:
            alerts.append({
                'type': 'temperature',
                'severity': 'critical',
                'message': "Temp %.1f°C" % m['temperature']
            })
        elif m['temperature'] > THRESHOLDS['temp_warning']:
            alerts.append({
                'type': 'temperature',
                'severity': 'warning',
                'message': "Temp %.1f°C" % m['temperature']
            })


        if m['core_diff'] > THRESHOLDS['core_diff_warning']:
            alerts.append({
                'type': 'core_diff',
                'severity': 'warning',
                'message': "Core diff %.1f%%" % m['core_diff']
            })

        if m['pps'] > THRESHOLDS['pps_critical']:
            alerts.append({
                'type': 'ddos_pps',
                'severity': 'critical',
                'message': "Аномальный PPS: %d пакетов/с (порог %d)" % (
                    m['pps'], THRESHOLDS['pps_critical']
                )
            })
        elif m['pps'] > THRESHOLDS['pps_warning']:
            alerts.append({
                'type': 'ddos_pps',
                'severity': 'warning',
                'message': "Повышенный PPS: %d пакетов/с" % m['pps']
            })


        if m['bps'] > THRESHOLDS['bps_critical']:
            alerts.append({
                'type': 'ddos_bps',
                'severity': 'critical',
                'message': "Аномальный BPS: %d бит/с (порог %d)" % (
                    m['bps'], THRESHOLDS['bps_critical']
                )
            })
        elif m['bps'] > THRESHOLDS['bps_warning']:
            alerts.append({
                'type': 'ddos_bps',
                'severity': 'warning',
                'message': "Повышенный BPS: %d бит/с" % m['bps']
            })

        return alerts

    def check_zscore(self, m, history):

        alerts = []


        if len(history) < 30:
            return alerts


        features = ['cpu', 'ram', 'disk', 'temperature', 'core_diff', 'pps', 'bps']


        feature_names = {
            'cpu': 'CPU', 'ram': 'RAM', 'disk': 'Disk',
            'temperature': 'Temp', 'core_diff': 'Core diff',
            'pps': 'PPS', 'bps': 'BPS'
        }


        z_threshold = ZSCORE_THRESHOLD


        for feat in features:

            hist_values = [h[feat] for h in history if feat in h]


            if len(hist_values) < 30:
                continue


            mean = np.mean(hist_values)

            std = np.std(hist_values)


            if std == 0:
                continue


            z_score = abs((m[feat] - mean) / std)


            if z_score > z_threshold:

                direction = "выше" if m[feat] > mean else "ниже"
                alerts.append({
                    'type': 'zscore_' + feat,
                    'severity': 'warning',
                    'message': "Z-score %s: %.1f (%s нормы)" % (
                        feature_names[feat], z_score, direction
                    )
                })

        return alerts

    def check_iqr(self, m, history):

        alerts = []

        if len(history) < 30:
            return alerts


        features = ['cpu', 'ram', 'disk', 'pps', 'bps']
        feature_names = {
            'cpu': 'CPU', 'ram': 'RAM', 'disk': 'Disk',
            'pps': 'PPS', 'bps': 'BPS'
        }

        for feat in features:

            hist_values = sorted([h[feat] for h in history if feat in h])

            if len(hist_values) < 30:
                continue


            q1 = np.percentile(hist_values, 25)
            q3 = np.percentile(hist_values, 75)
            iqr = q3 - q1


            upper = q3 + 1.5 * iqr


            if m[feat] > upper:
                alerts.append({
                    'type': 'iqr_' + feat,
                    'severity': 'warning',
                    'message': "IQR %s: %.1f > верхняя граница %.1f" % (
                        feature_names[feat], m[feat], upper
                    )
                })

        return alerts

    def check_ml(self, m):

        if not self.use_ml:
            return False, 0

        try:

            features = np.array([[
                m['cpu'],
                m['ram'],
                m['disk'],
                m['temperature'],
                m['core_diff'],
                m['pps'],
                m['bps']
            ]])


            features_scaled = self.scaler.transform(features)

            prediction = self.model.predict(features_scaled)[0]


            score = self.model.decision_function(features_scaled)[0]


            return prediction == -1, score

        except Exception as e:
            logger.error("Ошибка ML: %s", e)
            return False, 0


    def analyze(self, m):

        server_id = m['server_id']


        if m.get('_offline'):

            return {
                'server_id': server_id,
                'severity': 'critical',
                'alerts': [{
                    'type': 'offline',
                    'severity': 'critical',
                    'message': "Сервер %s не отправляет метрики > 60 сек" % server_id
                }],
                'ml_anomaly': False,
                'requires_isolation': False
            }


        rule_alerts = self.check_rules(m)


        history = self.get_historical_metrics(server_id, minutes=60)


        zscore_alerts = self.check_zscore(m, history)


        iqr_alerts = self.check_iqr(m, history)


        ml_anomaly, ml_score = self.check_ml(m)


        all_alerts = rule_alerts + zscore_alerts + iqr_alerts


        if ml_anomaly:
            all_alerts.append({
                'type': 'ml_anomaly',
                'severity': 'warning',
                'message': "ML-модель обнаружила аномалию (score: %.3f)" % ml_score
            })


        severity = 'normal'
        requires_isolation = False


        has_critical = any(a['severity'] == 'critical' for a in all_alerts)
        has_warning = any(a['severity'] == 'warning' for a in all_alerts)


        ddos_critical = any(
            a['severity'] == 'critical' and a['type'].startswith('ddos_')
            for a in all_alerts
        )

        if ddos_critical:

            severity = 'critical'
            requires_isolation = True
        elif has_critical:

            severity = 'critical'
        elif has_warning or ml_anomaly:

            severity = 'warning'


        return {
            'server_id': server_id,
            'severity': severity,
            'alerts': all_alerts,
            'ml_anomaly': ml_anomaly,
            'requires_isolation': requires_isolation
        }





    def _should_send(self, server_id, alert_type):

        now = time.time()
        last = self.last_alerts[server_id].get(alert_type, 0)


        if now - last < self.alert_cooldown:
            return False


        self.last_alerts[server_id][alert_type] = now
        return True

    def send_alert(self, result):



        if result['severity'] == 'normal':
            return

        server_id = result['server_id']


        for alert in result['alerts']:
            alert_type = alert['type']


            if not self._should_send(server_id, alert_type):
                continue


            payload = {
                'server_id': server_id,
                'severity': alert['severity'],
                'type': alert_type,
                'message': alert['message'],
                'timestamp': datetime.utcnow().isoformat() + 'Z',
                'requires_isolation': result.get('requires_isolation', False)
            }

            try:

                requests.post(API_URL + "/alerts", json=payload, timeout=3)
                logger.warning("Алерт отправлен: %s", payload)

            except Exception as e:
                logger.error("Ошибка отправки алерта: %s", e)


        if result.get('requires_isolation'):
            try:

                iso_reasons = [
                    a['message'] for a in result['alerts']
                    if a['type'].startswith('ddos_')
                ]


                iso_payload = {
                    'server_id': server_id,
                    'reason': 'DDoS-атака обнаружена: ' + '; '.join(iso_reasons),
                    'alerts': iso_reasons
                }


                resp