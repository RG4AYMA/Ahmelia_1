import json
import os
import re

import joblib
import numpy as np
from hot_retrain import penalize, reinforce, save
from sklearn.metrics.pairwise import cosine_similarity

BASE = os.path.dirname(os.path.abspath(__file__))
MODEL_DIR = os.path.join(BASE, "../_model")
MEMORY = os.path.join(MODEL_DIR, "memory.json")
ENCODER = os.path.join(MODEL_DIR, "encoder.joblib")

LOW_CONF = 0.35
HIGH_CONF = 0.75

with open(MEMORY, "r", encoding="utf-8") as f:
    memory = json.load(f)

vectorizer = joblib.load(ENCODER)

def embed(text):
    return vectorizer.transform([text]).toarray()[0]

def predict_intent(text):
    vec = embed(text)
    best, score = "UNKNOWN", 0.0

    for intent, data in memory["intents"].items():
        s = cosine_similarity([vec], [data["centroid"]])[0][0]
        if s > score:
            best, score = intent, s

    return (best if score >= LOW_CONF else "UNKNOWN"), score

def extract_entities(text):
    entities = {}
    keys = re.findall(r"([A-Za-z]+(\+[A-Za-z]+)+)", text)
    if keys:
        entities["keys"] = keys[0][0]

    words = text.split()
    if len(words) > 1:
        entities["app"] = " ".join(words[1:])

    return entities

if __name__ == "__main__":
    while True:
        text = input("\n>> ")
        if text.lower() == "exit":
            break

        intent, conf = predict_intent(text)
        entities = extract_entities(text)

        print(json.dumps({
            "intent": intent,
            "confidence": round(float(conf), 3),
            "entities": entities
        }, indent=2, ensure_ascii=False))

        # 🔁 AUTO-FEEDBACK
        if conf >= HIGH_CONF:
            reinforce(text, intent, weight=1.2)
            save()
            print("✔ auto-refuerzo")

        elif conf < LOW_CONF:
            fb = input("¿Correcto? (+1 / -1 / skip): ")
            if fb == "+1":
                reinforce(text, intent)
                save()
            elif fb == "-1":
                penalize(intent)
                correct = input("Intent correcto: ")
                reinforce(text, correct)
                save()
