package strategy

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func ParseCondition(input string) (Expression, error) {
	parser := conditionParser{input: input}
	expression, err := parser.parseExpression()
	if err != nil {
		return Expression{}, err
	}
	parser.skipSpace()
	if !parser.eof() {
		return Expression{}, fmt.Errorf("unexpected trailing input at %d", parser.pos)
	}
	return expression, nil
}

type conditionParser struct {
	input string
	pos   int
}

func (p *conditionParser) parseExpression() (Expression, error) {
	p.skipSpace()
	if p.eof() {
		return Expression{}, fmt.Errorf("unexpected end of condition")
	}
	if p.peekDigitOrSign() {
		return p.parseNumber()
	}
	ident, err := p.parseIdent()
	if err != nil {
		return Expression{}, err
	}
	switch ident {
	case "true":
		return Bool(true), nil
	case "false":
		return Bool(false), nil
	case "and", "or":
		args, err := p.parseCallArgs()
		if err != nil {
			return Expression{}, err
		}
		if len(args) < 2 {
			return Expression{}, fmt.Errorf("%s requires at least 2 args", ident)
		}
		if ident == "and" {
			return And(args...), nil
		}
		return Or(args...), nil
	case "not":
		args, err := p.parseCallArgs()
		if err != nil {
			return Expression{}, err
		}
		if len(args) != 1 {
			return Expression{}, fmt.Errorf("not requires 1 arg")
		}
		return Not(args[0]), nil
	default:
		return p.parseRefOrChain(ident)
	}
}

func (p *conditionParser) parseRefOrChain(first string) (Expression, error) {
	segments := []string{first}
	for p.consume(".") {
		segment, err := p.parseIdent()
		if err != nil {
			return Expression{}, err
		}
		segments = append(segments, segment)
		if p.nextNonSpaceIs("(") {
			method := segments[len(segments)-1]
			refSegments := segments[:len(segments)-1]
			return p.parseMethodCall(refSegments, method)
		}
	}
	return p.refFromSegments(segments), nil
}

func (p *conditionParser) parseMethodCall(refSegments []string, method string) (Expression, error) {
	args, err := p.parseCallArgs()
	if err != nil {
		return Expression{}, err
	}
	if len(refSegments) == 1 && refSegments[0] == "position" {
		return parsePositionStateMethod(method, args)
	}
	ref := p.refFromSegments(refSegments)
	switch method {
	case "turn_bullish", "turn_bearish":
		if len(args) != 0 {
			return Expression{}, fmt.Errorf("%s requires 0 args", method)
		}
		op := OperatorTurnBullish
		if method == "turn_bearish" {
			op = OperatorTurnBearish
		}
		return Expression{Kind: ExprOperator, Op: op, Args: []Expression{ref}}, nil
	case "gt", "gte", "lt", "lte", "eq", "cross_up", "cross_down":
		if len(args) != 1 {
			return Expression{}, fmt.Errorf("%s requires 1 arg", method)
		}
		op := map[string]Operator{
			"gt":         OperatorGT,
			"gte":        OperatorGTE,
			"lt":         OperatorLT,
			"lte":        OperatorLTE,
			"eq":         OperatorEQ,
			"cross_up":   OperatorCrossUp,
			"cross_down": OperatorCrossDown,
		}[method]
		return Expression{Kind: ExprOperator, Op: op, Args: []Expression{ref, args[0]}}, nil
	default:
		return Expression{}, fmt.Errorf("unknown chain method %q", method)
	}
}

func parsePositionStateMethod(method string, args []Expression) (Expression, error) {
	if len(args) != 0 {
		return Expression{}, fmt.Errorf("position.%s requires 0 args", method)
	}
	switch method {
	case "flat":
		return PositionFlat(), nil
	case "long":
		return PositionLong(), nil
	case "short":
		return PositionShort(), nil
	default:
		return Expression{}, fmt.Errorf("unknown position state method %q", method)
	}
}

func (p *conditionParser) refFromSegments(segments []string) Expression {
	if len(segments) >= 2 && segments[0] == "market" {
		return Market(segments[1]).Ref()
	}
	if len(segments) >= 2 && segments[0] == "position" {
		return Position(segments[1]).Ref()
	}
	if len(segments) >= 2 {
		return IndicatorField(segments[0], segments[1]).Ref()
	}
	return Indicator(segments[0]).Ref()
}

func (p *conditionParser) parseCallArgs() ([]Expression, error) {
	if !p.consume("(") {
		return nil, fmt.Errorf("expected '(' at %d", p.pos)
	}
	args := make([]Expression, 0)
	p.skipSpace()
	if p.consume(")") {
		return args, nil
	}
	for {
		expression, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		args = append(args, expression)
		p.skipSpace()
		if p.consume(")") {
			return args, nil
		}
		if !p.consume(",") {
			return nil, fmt.Errorf("expected ',' or ')' at %d", p.pos)
		}
	}
}

func (p *conditionParser) parseNumber() (Expression, error) {
	start := p.pos
	if p.peek() == '-' {
		p.pos++
	}
	for !p.eof() && (unicode.IsDigit(rune(p.peek())) || p.peek() == '.') {
		p.pos++
	}
	value, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		return Expression{}, fmt.Errorf("parse number %q: %w", p.input[start:p.pos], err)
	}
	return Number(value), nil
}

func (p *conditionParser) parseIdent() (string, error) {
	p.skipSpace()
	start := p.pos
	for !p.eof() {
		ch := rune(p.peek())
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
			p.pos++
			continue
		}
		break
	}
	if start == p.pos {
		return "", fmt.Errorf("expected identifier at %d", p.pos)
	}
	return p.input[start:p.pos], nil
}

func (p *conditionParser) consume(token string) bool {
	p.skipSpace()
	if strings.HasPrefix(p.input[p.pos:], token) {
		p.pos += len(token)
		return true
	}
	return false
}

func (p *conditionParser) nextNonSpaceIs(token string) bool {
	pos := p.pos
	for pos < len(p.input) && unicode.IsSpace(rune(p.input[pos])) {
		pos++
	}
	return strings.HasPrefix(p.input[pos:], token)
}

func (p *conditionParser) skipSpace() {
	for !p.eof() && unicode.IsSpace(rune(p.peek())) {
		p.pos++
	}
}

func (p *conditionParser) peekDigitOrSign() bool {
	ch := p.peek()
	return unicode.IsDigit(rune(ch)) || ch == '-'
}

func (p *conditionParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.input[p.pos]
}

func (p *conditionParser) eof() bool {
	return p.pos >= len(p.input)
}
