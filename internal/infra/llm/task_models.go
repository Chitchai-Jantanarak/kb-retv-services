package llm

import (
	"context"
	"fmt"

	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

type TaskModel struct {
	ConnectionID int64           `json:"connection_id"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	Name         string          `json:"name"`
	Default      bool            `json:"default"`
	Capabilities map[string]bool `json:"capabilities"`
}

type TaskModels struct {
	RouteVersion           int         `json:"route_version"`
	AllowModelSubstitution bool        `json:"allow_model_substitution"`
	Models                 []TaskModel `json:"models"`
}

func (r *CompanyResolver) TaskModels(ctx context.Context, companyID int64, task string) (TaskModels, error) {
	out := TaskModels{Models: []TaskModel{}}
	if cid, ok := ctxkey.CompanyID(ctx); !ok || cid <= 0 || cid != companyID {
		return out, apperr.New(apperr.CodeForbidden, "company context does not match")
	}
	if r.routes == nil {
		return out, nil
	}
	config, found, err := r.routes.RouteFor(ctx, companyID, task)
	if err != nil {
		return out, apperr.New(apperr.CodeUnavailable, "AI route unavailable")
	}
	if !found {
		return out, nil
	}
	if _, err := r.lookupAgent(ctx, companyID); err != nil {
		return out, apperr.New(apperr.CodeUnavailable, "AI is not enabled")
	}
	if !config.valid() {
		return out, apperr.New(apperr.CodeUnavailable, "AI route invalid")
	}
	out.RouteVersion, out.AllowModelSubstitution = config.Version, config.AllowModelSubstitution
	seen := map[string]bool{}
	for index, candidate := range config.Candidates {
		connection, err := r.routes.ConnectionFor(ctx, companyID, candidate.ConnectionID)
		if err != nil {
			return out, apperr.New(apperr.CodeUnavailable, "AI connection unavailable")
		}
		if !connection.Active {
			continue
		}
		for _, model := range connection.Models {
			key := fmt.Sprintf("%d:%s", connection.ID, model.ID)
			if seen[key] || !generationModel(model) {
				continue
			}
			seen[key] = true
			out.Models = append(out.Models, TaskModel{
				ConnectionID: connection.ID, Provider: connection.Provider, Model: model.ID, Name: model.Name,
				Default: index == 0 && model.ID == candidate.Model, Capabilities: model.Capabilities,
			})
		}
	}
	return out, nil
}

func generationModel(model ProviderModel) bool {
	supported, known := model.Capabilities["generate"]
	return !known || supported
}
