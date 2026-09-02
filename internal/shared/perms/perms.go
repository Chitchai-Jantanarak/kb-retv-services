package perms

import "strings"

var keys = []string{
	"console.access",
	"company.create",
	"company.update",
	"company.archive",
	"company.reparent",
	"company.admin",
	"report.view",
	"report.create",
	"report.update",
	"report.delete",
	"report.assign",
	"employee.view",
	"team.view",
	"customer.view",
	"site.view",
	"node.view",
	"kb.search",
	"portal.view",
}

func Keys() []string {
	out := make([]string, len(keys))
	copy(out, keys)
	return out
}

func Set() map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

func Valid(key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func Can(granted []string, required string) bool {
	if required == "" {
		return true
	}
	for _, g := range granted {
		if g == "*" || g == required {
			return true
		}
		if strings.HasSuffix(g, ".*") && strings.HasPrefix(required, strings.TrimSuffix(g, "*")) {
			return true
		}
	}
	return false
}
