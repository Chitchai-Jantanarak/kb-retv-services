import json
from pathlib import Path

import numpy as np
from model2vec import StaticModel
from sklearn.decomposition import PCA

OUT = Path(__file__).resolve().parents[4] / "assets" / "guard-embedder"
DIM = 128
BASE = "minishlab/potion-multilingual-128M"


def main() -> None:
    OUT.mkdir(parents=True, exist_ok=True)
    model = StaticModel.from_pretrained(BASE)

    vectors = np.asarray(model.embedding, dtype=np.float32)
    if vectors.shape[1] > DIM:
        vectors = PCA(n_components=DIM, whiten=False).fit_transform(vectors).astype(np.float32)
    vocab, dim = vectors.shape
    if dim != DIM:
        raise SystemExit(f"expected {DIM} dims, got {dim}")

    per_row = np.maximum(np.abs(vectors).max(axis=1), 1e-8) / 127.0
    q = np.clip(np.round(vectors / per_row[:, None]), -127, 127).astype(np.int8)

    (OUT / "model.int8.bin").write_bytes(q.tobytes(order="C"))
    (OUT / "scales.json").write_text(json.dumps({"per_row": per_row.tolist()}))

    unk = model.tokenizer.token_to_id("<unk>")
    (OUT / "meta.json").write_text(
        json.dumps(
            {
                "vocab": int(vocab),
                "dim": DIM,
                "unk_id": int(unk if unk is not None else 0),
                "calibrated": False,
            }
        )
    )
    model.tokenizer.save(str(OUT / "tokenizer.json"))

    size_mb = (OUT / "model.int8.bin").stat().st_size / 1e6
    print(f"wrote vocab={vocab} dim={dim} size={size_mb:.1f}MB")
    if size_mb > 100:
        raise SystemExit(f"asset {size_mb:.1f}MB exceeds 100MB budget")


if __name__ == "__main__":
    main()
