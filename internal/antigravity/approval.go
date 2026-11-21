package antigravity

import "strings"

// ApprovalDecision represents a parsed voice or keyboard decision on a pending tool permission request.
type ApprovalDecision string

const (
	DecisionNone    ApprovalDecision = ""
	DecisionApprove ApprovalDecision = "approve"
	DecisionDeny    ApprovalDecision = "deny"
)

// ParseVoiceApprovalIntent inspects a short spoken transcript when a permission prompt
// is active and classifies it as approve, deny, or unrecognized.
func ParseVoiceApprovalIntent(transcript string) ApprovalDecision {
	replacer := strings.NewReplacer(",", " ", ".", " ", "!", " ", "?", " ", ";", " ", ":", " ", "\"", " ")
	clean := strings.Join(strings.Fields(strings.ToLower(replacer.Replace(transcript))), " ")
	if clean == "" {
		return DecisionNone
	}

	approvePhrases := []string{
		"approve", "approved", "yes", "yeah", "yep", "yup", "allow",
		"go ahead", "do it", "proceed", "confirm", "accept", "ok", "okay", "sure",
	}
	for _, p := range approvePhrases {
		if clean == p || strings.HasPrefix(clean, p+" ") || strings.HasSuffix(clean, " "+p) {
			return DecisionApprove
		}
	}

	denyPhrases := []string{
		"deny", "denied", "no", "nope", "nah", "cancel", "reject",
		"stop", "abort", "don't", "do not", "block", "decline",
	}
	for _, p := range denyPhrases {
		if clean == p || strings.HasPrefix(clean, p+" ") || strings.HasSuffix(clean, " "+p) {
			return DecisionDeny
		}
	}

	return DecisionNone
}
