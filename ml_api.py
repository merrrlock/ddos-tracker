from fastapi import FastAPI
from pydantic import BaseModel
import joblib
import numpy as np

app = FastAPI()

model = joblib.load('models/isolation_forest.pkl')
scaler = joblib.load('models/scaler.pkl')

class MetricsBody(BaseModel):
    cpu: float
    ram: float
    disk: float = 0.0
    temperature: float = 0.0
    core_diff: float = 0.0
    pps: float = 0.0
    bps: float = 0.0

@app.post("/api/predict")
def predict_anomaly(m: MetricsBody):
    features = np.array([[m.cpu, m.ram, m.disk, m.temperature, m.core_diff, m.pps, m.bps]])

    features_scaled = scaler.transform(features)
    prediction = model.predict(features_scaled)[0]
    score = model.decision_function(features_scaled)[0]

    is_anomaly = bool(prediction == -1)

    return {
        "is_anomaly": is_anomaly,
        "score": float(score)
    }
