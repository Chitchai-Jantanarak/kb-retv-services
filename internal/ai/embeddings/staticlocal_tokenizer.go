//go:build tokenizers

package embeddings

import (
	"fmt"
	"path/filepath"

	"github.com/daulet/tokenizers"
)

type hfEncoder struct {
	tk *tokenizers.Tokenizer
}

func (e hfEncoder) EncodeIDs(text string) []uint32 {
	ids, _ := e.tk.Encode(text, false)
	return ids
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
	return &StaticLocalProvider{table: table, enc: hfEncoder{tk: tk}}, nil
}
