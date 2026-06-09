package qdrant

import "testing"

func TestCollectionForCompanyUsesCompanyCollection(t *testing.T) {
	got, err := CollectionForCompany("kb_chunks", 42)
	if err != nil {
		t.Fatalf("CollectionForCompany() error = %v", err)
	}
	if got != "kb_chunks__42" {
		t.Fatalf("CollectionForCompany() = %q, want kb_chunks__42", got)
	}
}

func TestCollectionForCompanyRejectsMissingCompany(t *testing.T) {
	if _, err := CollectionForCompany("kb_chunks", 0); err == nil {
		t.Fatal("CollectionForCompany() error = nil, want error")
	}
}

func TestCompanyPayload(t *testing.T) {
	payload := CompanyPayload(42)
	if payload["company_id"] != int64(42) {
		t.Fatalf("company_id = %v, want 42", payload["company_id"])
	}
}
