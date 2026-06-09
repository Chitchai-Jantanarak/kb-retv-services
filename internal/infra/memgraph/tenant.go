package memgraph

import "fmt"

func CompanyProps(companyID int64, props map[string]any) map[string]any {
	out := make(map[string]any, len(props)+1)
	for key, value := range props {
		out[key] = value
	}
	out["company_id"] = companyID
	return out
}

func ExternalID(companyID int64, kind string, id string) string {
	return fmt.Sprintf("%d:%s:%s", companyID, kind, id)
}
