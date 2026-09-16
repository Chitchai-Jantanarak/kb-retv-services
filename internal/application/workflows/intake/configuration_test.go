package intake_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/application/workflows/intake"
)

type fakeConfiguration struct {
	value     intake.Configuration
	err       error
	companyID int64
}

func (f *fakeConfiguration) ConfigurationFor(_ context.Context, companyID int64) (intake.Configuration, error) {
	f.companyID = companyID
	return f.value, f.err
}

func TestMailConfigurationControlsAssessmentAndCreation(t *testing.T) {
	for _, tt := range []struct {
		name          string
		ai, automatic bool
		calls         int
	}{
		{"disabled", false, true, 0},
		{"manual creation", true, false, 1},
		{"automatic creation", true, true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeProvider{json: `{"classification":"new_issue","problem_detail":"stuck","product":"Bella Bot"}`}
			config := &fakeConfiguration{value: intake.Configuration{AIEnabled: tt.ai, AutoCreateEnabled: tt.automatic, Instructions: "Use our branch terminology."}}
			extractor := newExtractor(t, provider, intake.WithConfiguration(config), intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}}))
			result, err := extractor.Extract(context.Background(), 7, intake.Signals{Sender: "customer@example.com", Subject: "Bella Bot broken", Body: "Bella Bot is stuck and needs service."})
			if err != nil {
				t.Fatal(err)
			}
			if config.companyID != 7 {
				t.Fatalf("company = %d", config.companyID)
			}
			if provider.calls != tt.calls {
				t.Fatalf("vendor calls = %d, want %d", provider.calls, tt.calls)
			}
			if result.AutoCreateDisabled != (!tt.ai || !tt.automatic) {
				t.Fatalf("creation disabled = %v", result.AutoCreateDisabled)
			}
			if tt.ai && (!strings.Contains(provider.prompt.System, "Use our branch terminology.") || !strings.Contains(provider.prompt.System, "Bella Bot")) {
				t.Fatal("tenant instructions or catalogue missing from prompt")
			}
			if !tt.ai && result.Status != intake.StatusUnknown {
				t.Fatalf("disabled status = %s", result.Status)
			}
		})
	}
}

func TestMailConfigurationFailureDoesNotCallVendor(t *testing.T) {
	provider := &fakeProvider{}
	config := &fakeConfiguration{err: errors.New("database unavailable")}
	_, err := newExtractor(t, provider, intake.WithConfiguration(config)).Extract(context.Background(), 7, intake.Signals{Body: "Broken product needs service"})
	if err == nil || provider.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, provider.calls)
	}
}

func TestRuleEvaluationIndependentOfAI(t *testing.T) {
	for _, tt := range []struct {
		name      string
		config    intake.Configuration
		wantScore bool
		calls     int
	}{
		{"rules without AI", intake.Configuration{AutoCreateEnabled: true}, true, 0},
		{"evaluation off", intake.Configuration{AIEnabled: true, AutoCreateEnabled: true, EvaluationDisabled: true}, false, 0},
		{"all engines off", intake.Configuration{RulesDisabled: true}, false, 0},
		{"AI without rule gate", intake.Configuration{AIEnabled: true, RulesDisabled: true}, false, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeProvider{json: `{"classification":"unclear"}`}
			extractor := newExtractor(t, provider, intake.WithConfiguration(&fakeConfiguration{value: tt.config}))
			result, err := extractor.Extract(context.Background(), 7, intake.Signals{Sender: "customer@example.com", Subject: "Printer broken", Body: "Printer is broken and needs service."})
			if err != nil {
				t.Fatal(err)
			}
			if (result.Score > 0) != tt.wantScore {
				t.Fatalf("score = %d", result.Score)
			}
			if provider.calls != tt.calls {
				t.Fatalf("calls = %d", provider.calls)
			}
			if !result.AutoCreateDisabled {
				t.Fatal("incomplete or disabled evaluation must not create a Ticket")
			}
		})
	}
}
