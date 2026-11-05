import json
import os

import joblib
import numpy as np

BASE = os.path.dirname(os.path.abspath(__file__))
MODEL_DIR = os.path.join(BASE, "../_model")
MEMORY = os.path.join(MODEL_DIR, "memory.json")
ENCODER = os.path.join(MODEL_DIR, "encoder.joblib")

with open(MEMORY, "r", encoding="utf-8") as f:
    memory = json.load(f)

vectorizer = joblib.load(ENCODER)

def embed(text):
    return vectorizer.transform([text]).toarray()[0]

def reinforce(text, intent, weight=1.0):
    vec = embed(text) * weight

    memory["intents"].setdefault(intent, {
        "examples": [],
        "centroid": vec.tolist()
    })

    memory["intents"][intent]["examples"].append(text)
    vectors = [embed(t) for t in memory["intents"][intent]["examples"]]
    memory["intents"][intent]["centroid"] = np.mean(vectors, axis=0).tolist()

def penalize(intent, factor=0.9):
    if intent in memory["intents"]:
        memory["intents"][intent]["centroid"] = (
            np.array(memory["intents"][intent]["centroid"]) * factor
        ).tolist()

def save():
    with open(MEMORY, "w", encoding="utf-8") as f:
        json.dump(memory, f, indent=2, ensure_ascii=False)
