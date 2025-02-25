package aqlprocessor

import (
	"fmt"

	"github.com/bsn-si/IPEHR-gateway/src/pkg/aqlprocessor/aqlparser"
	"github.com/bsn-si/IPEHR-gateway/src/pkg/errors"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

type PredicateType string

const (
	StandartPathPredicate   PredicateType = "STANDARD_PREDICATE"
	ArchetypedPathPredicate PredicateType = "ARCHETYPED_PREDICATE"
	NodePathPredicate       PredicateType = "NODE_PREDICATE"
)

type PathPredicate struct {
	Type PredicateType

	StandartPredicate *StandartPredicate
	NodePredicate     *NodePredicate
	Archetype         *ArchetypePathPredicate
}

type PathPredicateOperand struct {
	Primitive  *Primitive
	ObjectPath *ObjectPath
	Parameter  *Parameter
	IDCode     *string
	AtCode     *string
}

// standardPredicate:
// objectPath COMPARISON_OPERATOR pathPredicateOperand;
type StandartPredicate struct {
	ObjectPath  *ObjectPath
	CMPOperator ComparisionSymbol
	Operand     *PathPredicateOperand
}

type ComparisionSymbol string

const (
	SymNone ComparisionSymbol = "NONE"
	SymLT   ComparisionSymbol = "<"
	SymGT   ComparisionSymbol = ">"
	SymLE   ComparisionSymbol = "<="
	SymGE   ComparisionSymbol = ">="
	SymNe   ComparisionSymbol = "!="
	SymEQ   ComparisionSymbol = "="
)

func getComparisionSimbol(ctx antlr.TerminalNode) (ComparisionSymbol, error) {
	switch ComparisionSymbol(ctx.GetText()) {
	case SymEQ:
		return SymEQ, nil
	case SymNe:
		return SymNe, nil
	case SymGE:
		return SymGE, nil
	case SymLE:
		return SymLE, nil
	case SymGT:
		return SymGT, nil
	case SymLT:
		return SymLT, nil
	default:
		return SymNone, fmt.Errorf("Unexpected comparison operator: %v", ctx.GetText()) //nolint
	}
}

type ArchetypePathPredicate struct {
	ArchetypeHRID *string
	Parameter     *Parameter
}

type Parameter string

func getParameter(tn antlr.TerminalNode) (*Parameter, error) {
	p, err := getCode[Parameter](tn, "$")
	if err != nil {
		return nil, err
	}

	return &p, nil
}

func getPathPredicate(ctx *aqlparser.PathPredicateContext) (PathPredicate, error) {
	var result PathPredicate

	if ctx.StandardPredicate() != nil {
		sp, err := getStandartPredicate(ctx.StandardPredicate().(*aqlparser.StandardPredicateContext))
		if err != nil {
			return PathPredicate{}, errors.Wrap(err, "cannot process PathPredicate.StandartPredicate")
		}

		result = PathPredicate{
			Type:              StandartPathPredicate,
			StandartPredicate: sp,
		}
	} else if ctx.ArchetypePredicate() != nil {
		ap, err := getArchetypePredicate(ctx.ArchetypePredicate().(*aqlparser.ArchetypePredicateContext))
		if err != nil {
			return PathPredicate{}, errors.Wrap(err, "cannot process PathPredicate.ArchetypePredicate")
		}

		result = PathPredicate{
			Type:      ArchetypedPathPredicate,
			Archetype: ap,
		}
	} else if ctx.NodePredicate() != nil {
		np, err := getNodePredicate(ctx.NodePredicate().(*aqlparser.NodePredicateContext))
		if err != nil {
			return PathPredicate{}, errors.Wrap(err, "cannot process PathPredicate.NodePredicate")
		}

		result = PathPredicate{
			Type:          NodePathPredicate,
			NodePredicate: np,
		}
	} else {
		return PathPredicate{}, fmt.Errorf("unknown path predicate type: %s", ctx.GetText()) //nolint
	}

	return result, nil
}

func getStandartPredicate(ctx *aqlparser.StandardPredicateContext) (*StandartPredicate, error) {
	result := StandartPredicate{}

	if ctx.ObjectPath() != nil {
		op, err := getObjectPath(ctx.ObjectPath().(*aqlparser.ObjectPathContext))
		if err != nil {
			return nil, errors.Wrap(err, "cannot get ObjectPath")
		}

		result.ObjectPath = op
	}

	if ctx.COMPARISON_OPERATOR() != nil {
		symb, err := getComparisionSimbol(ctx.COMPARISON_OPERATOR())
		if err != nil {
			return nil, errors.Wrap(err, "cannot get ComparisionOperator")
		}

		result.CMPOperator = symb
	}

	if ctx.PathPredicateOperand() != nil {
		operand, err := getPathPredicateOperand(ctx.PathPredicateOperand().(*aqlparser.PathPredicateOperandContext))
		if err != nil {
			return nil, err
		}

		result.Operand = operand
	}

	return &result, nil
}

func getArchetypePredicate(ctx *aqlparser.ArchetypePredicateContext) (*ArchetypePathPredicate, error) {
	result := ArchetypePathPredicate{}

	if ctx.ARCHETYPE_HRID() != nil {
		result.ArchetypeHRID = toRef(ctx.ARCHETYPE_HRID().GetText())
	} else if ctx.PARAMETER() != nil {
		p, err := getParameter(ctx.PARAMETER())
		if err != nil {
			return nil, errors.Wrap(err, "cannot get ArchtypePredicate.PARAMETER")
		}

		result.Parameter = p
	} else {
		return nil, fmt.Errorf("unexpected archetype predicate: %v", ctx.GetText()) //nolint
	}

	return &result, nil
}

func getPathPredicateOperand(ctx *aqlparser.PathPredicateOperandContext) (*PathPredicateOperand, error) {
	result := PathPredicateOperand{}

	if ctx.Primitive() != nil {
		p, err := getPrimitive(ctx.Primitive().(*aqlparser.PrimitiveContext))
		if err != nil {
			return nil, errors.Wrap(err, "cannot get PathPredicateOverand.Primitive")
		}

		result.Primitive = &p
	} else if ctx.ObjectPath() != nil {
		op, err := getObjectPath(ctx.ObjectPath().(*aqlparser.ObjectPathContext))
		if err != nil {
			return nil, errors.Wrap(err, "cannot get PathPredicateOperand.ObjectPath")
		}
		result.ObjectPath = op
	} else if ctx.PARAMETER() != nil {
		p, err := getParameter(ctx.PARAMETER())
		if err != nil {
			return nil, errors.Wrap(err, "cannot get PathPredicateOperand.PARAMETER")
		}

		result.Parameter = p
	} else if ctx.AT_CODE() != nil {
		result.AtCode = toRef(ctx.AT_CODE().GetText())
	} else if ctx.ID_CODE() != nil {
		result.IDCode = toRef(ctx.ID_CODE().GetText())
	}

	return &result, nil
}

func toRef[T any](v T) *T {
	return &v
}
