// Package patterns provides orchestration patterns for agent coordination.
package patterns

import (
	"fmt"

	"github.com/agentapiary/apiary/internal/nectar"
	"github.com/agentapiary/apiary/pkg/apiary"
	"go.uber.org/zap"
)

// NewOrchestrator creates an orchestrator based on Hive pattern.
// Returns the appropriate pattern type based on the Hive's pattern specification.
// If PatternSegments is specified, returns a ComposedPattern that chains multiple patterns.
// Otherwise, falls back to the single Pattern field for backward compatibility.
func NewOrchestrator(hive *apiary.Hive, bus nectar.MessageBus, logger *zap.Logger) (interface{}, error) {
	// Check if pattern composition is specified
	if len(hive.Spec.PatternSegments) > 0 {
		return NewComposedPattern(hive, bus, logger), nil
	}

	// Fall back to single pattern for backward compatibility
	switch hive.Spec.Pattern {
	case "pipeline":
		return NewPipeline(hive, bus, logger), nil
	case "hierarchical":
		return NewHierarchical(hive, bus, logger), nil
	case "swarm":
		return NewSwarm(hive, bus, logger), nil
	case "event-driven":
		return NewEventDriven(hive, bus, logger), nil
	default:
		return nil, fmt.Errorf("unknown pattern: %s", hive.Spec.Pattern)
	}
}
