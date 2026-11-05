import json
import os

import joblib
import numpy as np
import pandas as pd
from sklearn.feature_extraction.text import TfidfVectorizer

BASE = os.path.dirname(os.path.abspath(__file__))
DATASET = os.path.join(BASE, "..", "data", "dataset.csv")
MODEL_DIR = os.path.join(BASE, "../_model")
MEMORY = os.path.join(MODEL_DIR, "memory.json")
ENCODER = os.path.join(MODEL_DIR, "encoder.joblib")

os.makedirs(MODEL_DIR, exist_ok=True)

df = pd.read_csv(DATASET)

vectorizer = TfidfVectorizer(analyzer="char", ngram_range=(3, 5))
X = vectorizer.fit_transform(df["text"]).toarray()

memory = {"intents": {}}

for text, intent in zip(df["text"], df["intent"]):
    vec = vectorizer.transform([text]).toarray()[0]
    memory["intents"].setdefault(intent, {"examples": [], "vectors": []})
    memory["intents"][intent]["examples"].append(text)
    memory["intents"][intent]["vectors"].append(vec.tolist())

for intent in memory["intents"]:
    vectors = np.array(memory["intents"][intent]["vectors"])
    memory["intents"][intent] = {
        "examples": memory["intents"][intent]["examples"],
        "centroid": np.mean(vectors, axis=0).tolist()
    }

with open(MEMORY, "w", encoding="utf-8") as f:
    json.dump(memory, f, indent=2, ensure_ascii=False)

joblib.dump(vectorizer, ENCODER)
print("✅ Modelo entrenado")
