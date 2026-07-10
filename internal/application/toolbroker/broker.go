package toolbroker

import (
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/toolhandlers"
	"github.com/my/app/internal/application/tools"
)

type Deps struct {
	FTS       toolhandlers.FTSSource
	Reports   toolhandlers.CasesRepo
	Inbound   toolhandlers.InboundRepo
	Promoter  toolhandlers.Promoter
	Employees toolhandlers.EmployeeRepo
	Customers toolhandlers.CustomerRepo
}

type Broker struct {
	handlers map[string]skeleton.Handler
}

func New(deps Deps) *Broker {
	handlers := map[string]skeleton.Handler{}
	if deps.FTS != nil {
		handlers["knowledge"] = toolhandlers.NewKnowledge(deps.FTS)
	}
	if deps.Reports != nil {
		handlers["reports.update"] = toolhandlers.NewUpdateCase(deps.Reports)
		handlers["reports.close"] = toolhandlers.NewCloseCase(deps.Reports)
		handlers["reports.assign"] = toolhandlers.NewAssignCase(deps.Reports)
		handlers["reports.find"] = toolhandlers.NewFindCases(deps.Reports)
		handlers["reports.track"] = toolhandlers.NewTrackCase(deps.Reports)
		handlers["reports.byProduct"] = toolhandlers.NewProductCases(deps.Reports)
	}
	if deps.Employees != nil {
		handlers["employee.status"] = toolhandlers.NewEmployeeStatus(deps.Employees)
		handlers["workload"] = toolhandlers.NewWorkload(deps.Employees)
	}
	if deps.Customers != nil {
		handlers["customer.profile"] = toolhandlers.NewCustomerProfile(deps.Customers)
	}
	if deps.Inbound != nil {
		handlers["inbound.read"] = toolhandlers.NewInboundRead(deps.Inbound)
	}
	if deps.Promoter != nil {
		handlers["promote.mail"] = toolhandlers.NewPromoteMail(deps.Promoter)
	}
	return &Broker{handlers: handlers}
}

func (b *Broker) Handlers() map[string]skeleton.Handler {
	return b.handlers
}

func (b *Broker) Bound(catalog []tools.Tool) []tools.Tool {
	out := make([]tools.Tool, 0, len(catalog))
	for _, tool := range catalog {
		if _, ok := b.handlers[tool.Handler]; ok {
			out = append(out, tool)
		}
	}
	return out
}

func (b *Broker) UnboundIDs(catalog []tools.Tool) []string {
	var out []string
	for _, tool := range catalog {
		if _, ok := b.handlers[tool.Handler]; !ok {
			out = append(out, tool.ID)
		}
	}
	return out
}
