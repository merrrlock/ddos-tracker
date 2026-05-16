
import os
from dotenv import load_dotenv



load_dotenv(override=False)






DB_HOST = os.getenv('DB_HOST', 'localhost')
DB_PORT = int(os.getenv('DB_PORT', '5432'))
DB_NAME = os.getenv('DB_NAME', 'monitoring')
DB_USER = os.getenv('DB_USER', 'postgres')
DB_PASSWORD = os.getenv('DB_PASSWORD', '')






REDIS_HOST = os.getenv('REDIS_HOST', 'localhost')


REDIS_PORT = int(os.getenv('REDIS_PORT', 6379))


REDIS_ALERT_CHANNEL = os.getenv('REDIS_ALERT_CHANNEL', 'alerts_channel')





MODEL_DIR = os.getenv('MODEL_DIR', '/app/models')


MODEL_PATH = os.getenv('MODEL_PATH', MODEL_DIR + '/isolation_forest.pkl')


SCALER_PATH = os.getenv('SCALER_PATH', MODEL_DIR + '/scaler.pkl')







CHECK_INTERVAL = int(os.getenv('CHECK_INTERVAL', '5'))

ALERT_COOLDOWN = int(os.getenv('ALERT_COOLDOWN', '60'))

METRICS_WINDOW_SECONDS = int(os.getenv('METRICS_WINDOW_SECONDS', '15'))




LOG_LEVEL = os.getenv('LOG_LEVEL', 'INFO')
LOG_FILE = os.getenv('LOG_FILE', '/var/log/detector.log')



THRESHOLDS = {

    'cpu_warning': float(os.getenv('CPU_WARNING', '70')),
    'cpu_critical': float(os.getenv('CPU_CRITICAL', '90')),



    'ram_warning': float(os.getenv('RAM_WARNING', '80')),
    'ram_critical': float(os.getenv('RAM_CRITICAL', '90')),



    'disk_warning': float(os.getenv('DISK_WARNING', '80')),
    'disk_critical': float(os.getenv('DISK_CRITICAL', '90')),



    'temp_warning': float(os.getenv('TEMP_WARNING', '70')),
    'temp_critical': float(os.getenv('TEMP_CRITICAL', '85')),



    'core_diff_warning': float(os.getenv('CORE_DIFF_WARNING', '50')),



    'pps_warning': int(os.getenv('PPS_WARNING', '10000')),
    'pps_critical': int(os.getenv('PPS_CRITICAL', '50000')),



    'bps_warning': int(os.getenv('BPS_WARNING', '100000000')),
    'bps_critical': int(os.getenv('BPS_CRITICAL', '500000000')),

}



ZSCORE_THRESHOLD = float(os.getenv('ZSCORE_THRESHOLD', '3.0'))
TRAINING_DAYS = int(os.getenv('TRAINING_DAYS', '14'))
MIN_TRAINING_SAMPLES = int(os.getenv('MIN_TRAINING_SAMPLES', '1000'))
ML_FEATURES = ['cpu', 'ram', 'disk', 'temperature', 'core_diff', 'pps', 'bps']
