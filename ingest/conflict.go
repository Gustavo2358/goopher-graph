package ingest

import (
	"fmt"
	"gophergraph/ingest/ports"
)

type propertyDecision uint8

const (
	propertyKeep propertyDecision = iota
	propertyDrop
	propertyWinner
)

type propertySummary struct {
	cards    uint8
	distinct bool
	count    uint64
	origins  []ports.Location
	winner   ports.Location
}

// Both backends decide after reading the complete group, so cardinality
// conflicts take precedence and a later duplicate cannot resurrect a drop.
func (b *builder) decideProperty(role ports.Role, id string, s propertySummary) (propertyDecision, error) {
	code := ""
	if s.cards == 3 {
		code = "PROPERTY_CARDINALITY_CONFLICT"
		b.report.PropertyCardinalityConflictGroups++
	} else if s.cards == 2 && s.distinct {
		if role == ports.Nodes && b.policy != DropConflictingProperty {
			b.report.ResolvedNodePropertyConflictGroups++
			return propertyWinner, b.emit(ports.Diagnostic{
				Severity: ports.Warning, Code: "NODE_PROPERTY_CONFLICT_RESOLVED",
				Location: s.winner, Related: s.origins, Contributions: s.count,
				EntityID: id, EntityIDKnown: true, Resolution: b.policy.String(),
				Message: fmt.Sprintf("property conflict resolved by %s after %d contributions", b.policy, s.count),
			})
		}
		code = "PROPERTY_CONFLICT"
		b.report.PropertyConflictGroups++
	}
	if code == "" {
		return propertyKeep, nil
	}
	return propertyDrop, b.emit(ports.Diagnostic{
		Severity: ports.Rejection, Code: code, Location: s.origins[0],
		Related: s.origins, Contributions: s.count, EntityID: id, EntityIDKnown: true,
		Message: fmt.Sprintf("property removed after %d contributions", s.count),
	})
}
