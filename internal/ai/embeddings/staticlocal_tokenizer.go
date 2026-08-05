//go:build tokenizers

package embeddings

import (
	"fmt"
	"path/filepath"

	"github.com/daulet/tokenizers"
)

type hfEncoder struct {
	tk    *tokenizers.Tokenizer
	vocab int
}

func (e hfEncoder) EncodeIDs(text string) ([]uint32, error) {
	ids, _, err := e.tk.EncodeErr(text, false)
	if err != nil {
		return nil, fmt.Errorf("guard embedder: encode: %w", err)
	}
	for _, id := range ids {
		if int(id) >= e.vocab {
			return nil, fmt.Errorf("guard embedder: tokenizer emitted id %d beyond vocab %d; tokenizer.json does not match model.int8.bin", id, e.vocab)
		}
	}
	return ids, nil
}

func NewStaticLocalProvider(dir string) (*StaticLocalProvider, error) {
	table, err := loadStaticTable(dir)
	if err != nil {
		return nil, err
	}
	tk, err := tokenizers.FromFile(filepath.Join(dir, "tokenizer.json"))
	if err != nil {
		return nil, fmt.Errorf("guard embedder: load tokenizer: %w", err)
	}
	return &StaticLocalProvider{table: table, enc: hfEncoder{tk: tk, vocab: table.vocab}}, nil
}
