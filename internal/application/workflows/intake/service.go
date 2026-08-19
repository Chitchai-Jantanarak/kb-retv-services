package intake

import (
	"context"

	"go.uber.org/zap"
)

type Sink interface {
	WriteCompleteness(ctx context.Context, conversationID int64, r Result) error
}

type Service struct {
	extractor *Extractor
	sink      Sink
	log       *zap.Logger
}

func NewService(extractor *Extractor, sink Sink, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{extractor: extractor, sink: sink, log: log}
}

func (s *Service) Assess(ctx context.Context, companyID, conversationID int64, sig Signals) (Result, error) {
	if s == nil || s.extractor == nil {
		return Result{Status: StatusUnknown}, nil
	}

	result, err := s.extractor.Extract(ctx, companyID, sig)
	if err != nil {
		s.log.Warn("intake completeness extraction failed",
			zap.Int64("company_id", companyID),
			zap.Int64("conversation_id", conversationID),
			zap.Error(err))
		return Result{Status: StatusUnknown}, err
	}

	result.Confidence, result.ConfidenceReasons = Confidence(
		result.Score, result.Reasons, result.Classification,
		result.Missing, result.CatalogRelated, result.ReferencedCase,
	)

	if s.sink != nil {
		if err := s.sink.WriteCompleteness(ctx, conversationID, result); err != nil {
			s.log.Warn("intake completeness persist failed",
				zap.Int64("company_id", companyID),
				zap.Int64("conversation_id", conversationID),
				zap.Error(err))
			return result, err
		}
	}

	s.log.Info("intake completeness assessed",
		zap.Int64("company_id", companyID),
		zap.Int64("conversation_id", conversationID),
		zap.String("status", result.Status),
		zap.Strings("missing", result.Missing))
	return result, nil
}
