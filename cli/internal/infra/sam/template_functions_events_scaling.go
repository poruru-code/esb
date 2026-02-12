// Where: cli/internal/infra/sam/template_functions_events_scaling.go
// What: Event and scaling extraction helpers for parsed functions.
// Why: Keep trigger/scaling parsing rules independent from resource decoding.
package sam

import (
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
)

func parseEvents(events map[string]any) []template.EventSpec {
	if events == nil {
		return nil
	}
	result := []template.EventSpec{}
	for _, eventName := range sortedMapKeys(events) {
		raw := events[eventName]
		event := value.AsMap(raw)
		if event == nil {
			continue
		}
		eventType := value.AsString(event["Type"])
		props := value.AsMap(event["Properties"])
		if props == nil {
			continue
		}

		switch eventType {
		case "Api":
			path := value.AsString(props["Path"])
			method := value.AsString(props["Method"])
			if path == "" || method == "" {
				continue
			}
			result = append(result, template.EventSpec{
				Type:   "Api",
				Path:   path,
				Method: strings.ToLower(method),
			})
		case "Schedule":
			schedule := value.AsString(props["Schedule"])
			if schedule == "" {
				continue
			}
			input := value.AsString(props["Input"])
			result = append(result, template.EventSpec{
				Type:               "Schedule",
				ScheduleExpression: schedule,
				Input:              input,
			})
		}
	}
	return result
}

func parseScaling(props map[string]any) template.ScalingSpec {
	var scaling template.ScalingSpec
	if value, ok := value.AsIntPointer(props["ReservedConcurrentExecutions"]); ok {
		scaling.MaxCapacity = value
	}
	if provisioned := value.AsMap(props["ProvisionedConcurrencyConfig"]); provisioned != nil {
		if value, ok := value.AsIntPointer(provisioned["ProvisionedConcurrentExecutions"]); ok {
			scaling.MinCapacity = value
		}
	}
	return scaling
}
