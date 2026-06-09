package qdrant

import (
	"errors"
	"fmt"
)

func CollectionForCompany(prefix string, companyID int64) (string, error) {
	if prefix == "" {
		return "", errors.New("collection prefix is required")
	}
	if companyID <= 0 {
		return "", errors.New("company_id must be positive")
	}
	return fmt.Sprintf("%s__%d", prefix, companyID), nil
}

func CompanyPayload(companyID int64) map[string]any {
	return map[string]any{
		"company_id": companyID,
	}
}
