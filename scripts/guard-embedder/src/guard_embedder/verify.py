import json
import os
from pathlib import Path

import numpy as np
from tokenizers import Tokenizer

OUT = Path(__file__).resolve().parents[4] / "assets" / "guard-embedder"
SCRIPT_DIR = Path(__file__).resolve().parents[2]
REPO_ROOT = Path(__file__).resolve().parents[4]


def data_path(env_name: str, real_path: Path, example_path: Path) -> Path:
    override = os.environ.get(env_name)
    if override:
        return Path(override)
    if real_path.exists():
        return real_path
    return example_path


PROBES = data_path("GUARD_PROBES_PATH", SCRIPT_DIR / "probes.json", SCRIPT_DIR / "probes.json.example")
ANCHORS = data_path(
    "GUARD_ANCHORS_PATH",
    REPO_ROOT / "internal" / "application" / "intent" / "anchors.json",
    REPO_ROOT / "internal" / "application" / "intent" / "anchors.json.example",
)


def load():
    meta = json.loads((OUT / "meta.json").read_text(encoding="utf-8"))
    vocab, dim = meta["vocab"], meta["dim"]
    q = np.frombuffer((OUT / "model.int8.bin").read_bytes(), dtype=np.int8).reshape(vocab, dim)
    scale = np.asarray(
        json.loads((OUT / "scales.json").read_text(encoding="utf-8"))["per_row"], dtype=np.float32
    )
    table = q.astype(np.float32) * scale[:, None]
    tok = Tokenizer.from_file(str(OUT / "tokenizer.json"))
    return meta, table, tok


def encode_ids(tok, text, vocab, unk):
    ids = [i for i in tok.encode(text, add_special_tokens=False).ids if 0 <= i < vocab]
    return ids or [unk]


def embed(table, ids):
    v = table[ids].mean(axis=0)
    n = float(np.linalg.norm(v))
    return v / n if n else v


def decide(e, pos, neg, floor, margin):
    bp = max(float(e @ a) for a in pos)
    bn = max(float(e @ a) for a in neg)
    return "off_domain" if (bn >= bp + margin or bp < floor) else "in_domain"


def main() -> None:
    meta, table, tok = load()
    vocab, unk = meta["vocab"], meta["unk_id"]
    data = json.loads(PROBES.read_text(encoding="utf-8"))
    anchors = json.loads(ANCHORS.read_text(encoding="utf-8"))
    positive = [a for key, group in anchors.items() if key != "off_domain" for a in group]
    pos = [embed(table, encode_ids(tok, t, vocab, unk)) for t in positive]
    neg = [embed(table, encode_ids(tok, t, vocab, unk)) for t in anchors["off_domain"]]

    floor, accept = 0.35, 0.55
    margin = None
    for m in (0.05, 0.08, 0.12):
        wrong = [
            p
            for p in data["probes"]
            if decide(embed(table, encode_ids(tok, p["text"], vocab, unk)), pos, neg, floor, m)
            != p["label"]
        ]
        if not wrong:
            margin = m
            break
    if margin is None:
        bad = [w["text"] for w in wrong]
        raise SystemExit(f"probes still misclassified at margin sweep: {bad}")

    meta.update({"floor": floor, "accept": accept, "margin": margin, "calibrated": True})
    (OUT / "meta.json").write_text(json.dumps(meta), encoding="utf-8")

    fixtures = []
    for p in data["probes"][:6]:
        ids = encode_ids(tok, p["text"], vocab, unk)
        fixtures.append({"text": p["text"], "token_ids": ids, "vec128": embed(table, ids).tolist()})
    (OUT / "fixtures.json").write_text(json.dumps(fixtures), encoding="utf-8")
    print(f"gate passed; margin={margin}; wrote thresholds + {len(fixtures)} fixtures")


if __name__ == "__main__":
    main()
