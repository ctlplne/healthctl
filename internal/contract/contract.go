// Package contract holds the limits of the model proposal contract. The
// clinical validator enforces them and the provider prompt states them, from
// the same constants, so the two can't drift apart.
package contract

import "fmt"

const (
	MaxChoices           = 8
	MaxChoiceBytes       = 160
	MaxOutstanding       = 16
	MaxOutstandingBytes  = 300
	MaxFindings          = 32
	MaxUrgent            = 8
	MaxItems             = 8
	MaxQuestionBytes     = 500
	MaxFreeQuestionBytes = 300
	MaxReasonBytes       = 700
	MaxConceptBytes      = 120
	MaxTimeBytes         = 180
	MaxValueBytes        = 120
	MaxUnitsBytes        = 64
	MaxItemNameBytes     = 120
	MaxExplanationBytes  = 800
	MaxLimitations       = 8
	MaxLimitationBytes   = 300
	MaxReferences        = 12
)

// LimitsText is appended to the system prompt.
func LimitsText() string {
	return fmt.Sprintf("Application limits for every call: at most %d choices of up to %d bytes, %d outstanding questions of up to %d bytes, %d findings and %d urgentEvidence entries; a question of up to %d bytes with a reason of up to %d bytes; a concept of up to %d bytes, a timeContext of up to %d bytes, a value of up to %d bytes and units of up to %d bytes. An assessment has at most %d differential items, each with a name of up to %d bytes, an explanation of up to %d bytes, 1 to %d supporting and up to %d conflicting fact IDs, and 1 to %d limitations of up to %d bytes.",
		MaxChoices, MaxChoiceBytes, MaxOutstanding, MaxOutstandingBytes, MaxFindings, MaxUrgent, MaxQuestionBytes, MaxReasonBytes, MaxConceptBytes, MaxTimeBytes, MaxValueBytes, MaxUnitsBytes,
		MaxItems, MaxItemNameBytes, MaxExplanationBytes, MaxReferences, MaxReferences, MaxLimitations, MaxLimitationBytes)
}
