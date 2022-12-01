package template

import (
	"context"
	"hms/gateway/pkg/docs/model"
	"hms/gateway/pkg/docs/parser/adl14"
	"hms/gateway/pkg/docs/parser/adl2"
	"hms/gateway/pkg/docs/service"
	"hms/gateway/pkg/errors"
)

type Service struct {
	*service.DefaultDocumentService
	parsers map[string]ADLParser
}

type ADLParser interface {
	Version() model.ADLVer
	IsTypeAllowed(t model.ADLType) bool
	Validate([]byte, model.ADLType) bool
}

func NewService(docService *service.DefaultDocumentService) *Service {
	ps := make(map[string]ADLParser)

	opt14 := adl14.NewParser()
	opt2 := adl2.NewParser()

	ps[opt14.Version()] = opt14
	ps[opt2.Version()] = opt2

	return &Service{
		docService,
		ps,
	}
}

func (s *Service) Parser(version string) (ADLParser, error) {
	p, ok := s.parsers[version]
	if !ok {
		return nil, errors.ErrIsNotExist
	}

	return p, nil
}

func (*Service) GetByID(ctx context.Context, userID string, templateID string) (*model.Template, error) {
	return nil, errors.ErrNotImplemented
}
