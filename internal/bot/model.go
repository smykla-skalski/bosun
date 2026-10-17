package bot

import (
	"context"

	"github.com/smykla-skalski/bosun/internal/messageprofile"
	"github.com/smykla-skalski/bosun/internal/openrouter"
	"github.com/smykla-skalski/bosun/internal/replies"
)

const (
	replySourceOpenRouter = "openrouter"
	replySourceStatic     = "static"
	noFault               = "none"
)

type replyResult struct {
	text           string
	source         string
	faultClass     string
	selectionIndex int
}

func (s *Service) resolveReply(
	ctx context.Context,
	memberText string,
	trigger TriggerType,
	category replies.Category,
	selections [replies.CandidateCount]replies.Selection,
) replyResult {
	fallback := selections[0]
	if s.model == nil {
		return staticResult(fallback.Reply.Text, noFault)
	}
	profile, err := messageprofile.Build(memberText, s.botHandle, string(trigger), string(category), tierName(fallback.Tier))
	if err != nil {
		return staticResult(fallback.Reply.Text, openrouter.FaultInvalidInput)
	}
	input := openrouter.Input{Profile: profile, CandidateType: string(category)}
	for index, selection := range selections {
		if err := replies.ValidateModelCandidate(selection.Reply.Text, s.botHandle); err != nil {
			return staticResult(fallback.Reply.Text, openrouter.FaultInvalidInput)
		}
		input.Candidates[index] = selection.Reply.Text
	}
	choice, err := s.model.Complete(ctx, input)
	if err != nil {
		return staticResult(fallback.Reply.Text, openrouter.SafeFaultClass(err))
	}
	if choice < 0 || choice >= replies.CandidateCount {
		return staticResult(fallback.Reply.Text, openrouter.FaultInvalidOutput)
	}
	return replyResult{
		text:           selections[choice].Reply.Text,
		source:         replySourceOpenRouter,
		faultClass:     noFault,
		selectionIndex: choice,
	}
}

func tierName(tier replies.Tier) string {
	switch tier {
	case replies.TierSharp:
		return "sharp"
	case replies.TierHarsh:
		return "harsh"
	case replies.TierSavage:
		return "savage"
	default:
		return "unknown"
	}
}

func staticResult(text string, faultClass string) replyResult {
	return replyResult{
		text:       text,
		source:     replySourceStatic,
		faultClass: faultClass,
	}
}

func (r replyResult) withFault(faultClass string) replyResult {
	r.faultClass = faultClass
	return r
}

func (s *Service) logResult(selection replies.Selection, result replyResult) {
	s.logger.Info("reply result",
		"source", result.source,
		"tier", selection.Tier,
		"static_reply_id", selection.Reply.ID,
		"fault_class", result.faultClass,
	)
}

func (s *Service) logFault(selection replies.Selection, result replyResult) {
	s.logger.Warn("reply fault",
		"source", result.source,
		"tier", selection.Tier,
		"static_reply_id", selection.Reply.ID,
		"fault_class", result.faultClass,
	)
}
