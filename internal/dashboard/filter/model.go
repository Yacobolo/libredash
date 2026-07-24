package filter

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ValueKind string

const (
	ValueString    ValueKind = "string"
	ValueBoolean   ValueKind = "boolean"
	ValueInteger   ValueKind = "integer"
	ValueDecimal   ValueKind = "decimal"
	ValueDate      ValueKind = "date"
	ValueTimestamp ValueKind = "timestamp"
)

type Value struct {
	Kind  ValueKind `json:"kind" yaml:"kind"`
	Value any       `json:"value" yaml:"value"`
}

type ExpressionKind string

const (
	ExpressionUnfiltered     ExpressionKind = "unfiltered"
	ExpressionNullCheck      ExpressionKind = "null_check"
	ExpressionSet            ExpressionKind = "set"
	ExpressionComparison     ExpressionKind = "comparison"
	ExpressionRange          ExpressionKind = "range"
	ExpressionRelativePeriod ExpressionKind = "relative_period"
)

type Operator string

const (
	OperatorIsNull             Operator = "is_null"
	OperatorIsNotNull          Operator = "is_not_null"
	OperatorIn                 Operator = "in"
	OperatorNotIn              Operator = "not_in"
	OperatorEquals             Operator = "equals"
	OperatorNotEquals          Operator = "not_equals"
	OperatorContains           Operator = "contains"
	OperatorNotContains        Operator = "not_contains"
	OperatorStartsWith         Operator = "starts_with"
	OperatorEndsWith           Operator = "ends_with"
	OperatorGreaterThan        Operator = "greater_than"
	OperatorGreaterThanOrEqual Operator = "greater_than_or_equal"
	OperatorLessThan           Operator = "less_than"
	OperatorLessThanOrEqual    Operator = "less_than_or_equal"
)

type Bound struct {
	Value     Value `json:"value" yaml:"value"`
	Inclusive bool  `json:"inclusive" yaml:"inclusive"`
}

type RelativeDirection string

const (
	DirectionPrevious RelativeDirection = "previous"
	DirectionCurrent  RelativeDirection = "current"
	DirectionNext     RelativeDirection = "next"
)

type RelativeUnit string

const (
	UnitMinute  RelativeUnit = "minute"
	UnitHour    RelativeUnit = "hour"
	UnitDay     RelativeUnit = "day"
	UnitWeek    RelativeUnit = "week"
	UnitMonth   RelativeUnit = "month"
	UnitQuarter RelativeUnit = "quarter"
	UnitYear    RelativeUnit = "year"
)

type RelativeAnchor string

const (
	AnchorCurrentTime    RelativeAnchor = "current_time"
	AnchorFirstAvailable RelativeAnchor = "first_available"
	AnchorLastAvailable  RelativeAnchor = "last_available"
	AnchorFixed          RelativeAnchor = "fixed"
)

type Expression struct {
	Kind           ExpressionKind    `json:"kind" yaml:"kind"`
	Operator       Operator          `json:"operator,omitempty" yaml:"operator,omitempty"`
	Values         []Value           `json:"values,omitempty" yaml:"values,omitempty"`
	Value          *Value            `json:"value,omitempty" yaml:"value,omitempty"`
	Lower          *Bound            `json:"lower,omitempty" yaml:"lower,omitempty"`
	Upper          *Bound            `json:"upper,omitempty" yaml:"upper,omitempty"`
	Direction      RelativeDirection `json:"direction,omitempty" yaml:"direction,omitempty"`
	Count          int               `json:"count,omitempty" yaml:"count,omitempty"`
	Unit           RelativeUnit      `json:"unit,omitempty" yaml:"unit,omitempty"`
	IncludeCurrent bool              `json:"includeCurrent,omitempty" yaml:"include_current,omitempty"`
	Anchor         RelativeAnchor    `json:"anchor,omitempty" yaml:"anchor,omitempty"`
	AnchorValue    *Value            `json:"anchorValue,omitempty" yaml:"anchor_value,omitempty"`
}

func (expression Expression) MarshalJSON() ([]byte, error) {
	type expressionJSON Expression
	encoded, err := json.Marshal(expressionJSON(expression))
	if err != nil || expression.Kind != ExpressionRelativePeriod || expression.IncludeCurrent {
		return encoded, err
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, err
	}
	object["includeCurrent"] = false
	return json.Marshal(object)
}

type Scope string

const (
	ScopeReport Scope = "report"
	ScopePage   Scope = "page"
)

var decimalPattern = regexp.MustCompile(`^[+-]?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)

func Canonicalize(expression Expression, valueKind ValueKind) (Expression, error) {
	if expression.Kind == "" || expression.Kind == ExpressionUnfiltered {
		return Expression{Kind: ExpressionUnfiltered}, nil
	}
	if !validValueKind(valueKind) {
		return Expression{}, fmt.Errorf("unsupported filter value kind %q", valueKind)
	}
	switch expression.Kind {
	case ExpressionNullCheck:
		if expression.Operator != OperatorIsNull && expression.Operator != OperatorIsNotNull {
			return Expression{}, fmt.Errorf("null_check has unsupported operator %q", expression.Operator)
		}
		return Expression{Kind: expression.Kind, Operator: expression.Operator}, nil
	case ExpressionSet:
		if expression.Operator != OperatorIn && expression.Operator != OperatorNotIn {
			return Expression{}, fmt.Errorf("set has unsupported operator %q", expression.Operator)
		}
		if len(expression.Values) == 0 {
			return Expression{Kind: ExpressionUnfiltered}, nil
		}
		values, err := canonicalValues(expression.Values, valueKind)
		if err != nil {
			return Expression{}, err
		}
		return Expression{Kind: expression.Kind, Operator: expression.Operator, Values: values}, nil
	case ExpressionComparison:
		if !comparisonOperatorAllowed(expression.Operator, valueKind) {
			return Expression{}, fmt.Errorf("comparison operator %q is not valid for %q", expression.Operator, valueKind)
		}
		if expression.Value == nil {
			return Expression{}, fmt.Errorf("comparison requires a value")
		}
		value, err := canonicalValue(*expression.Value, valueKind)
		if err != nil {
			return Expression{}, err
		}
		return Expression{Kind: expression.Kind, Operator: expression.Operator, Value: &value}, nil
	case ExpressionRange:
		if expression.Lower == nil && expression.Upper == nil {
			return Expression{Kind: ExpressionUnfiltered}, nil
		}
		if valueKind == ValueBoolean {
			return Expression{}, fmt.Errorf("range is not valid for %q", valueKind)
		}
		result := Expression{Kind: ExpressionRange}
		if expression.Lower != nil {
			value, err := canonicalValue(expression.Lower.Value, valueKind)
			if err != nil {
				return Expression{}, fmt.Errorf("lower bound: %w", err)
			}
			result.Lower = &Bound{Value: value, Inclusive: expression.Lower.Inclusive}
		}
		if expression.Upper != nil {
			value, err := canonicalValue(expression.Upper.Value, valueKind)
			if err != nil {
				return Expression{}, fmt.Errorf("upper bound: %w", err)
			}
			result.Upper = &Bound{Value: value, Inclusive: expression.Upper.Inclusive}
		}
		if result.Lower != nil && result.Upper != nil && compareValues(result.Lower.Value, result.Upper.Value) > 0 {
			return Expression{}, fmt.Errorf("range lower bound exceeds upper bound")
		}
		return result, nil
	case ExpressionRelativePeriod:
		return canonicalRelativePeriod(expression, valueKind)
	default:
		return Expression{}, fmt.Errorf("unsupported filter expression kind %q", expression.Kind)
	}
}

func canonicalRelativePeriod(expression Expression, valueKind ValueKind) (Expression, error) {
	if valueKind != ValueDate && valueKind != ValueTimestamp {
		return Expression{}, fmt.Errorf("relative_period is not valid for %q", valueKind)
	}
	if expression.Direction != DirectionPrevious && expression.Direction != DirectionCurrent && expression.Direction != DirectionNext {
		return Expression{}, fmt.Errorf("relative_period has unsupported direction %q", expression.Direction)
	}
	if expression.Count <= 0 {
		return Expression{}, fmt.Errorf("relative_period count must be positive")
	}
	switch expression.Unit {
	case UnitMinute, UnitHour:
		if valueKind != ValueTimestamp {
			return Expression{}, fmt.Errorf("relative_period unit %q requires timestamp values", expression.Unit)
		}
	case UnitDay, UnitWeek, UnitMonth, UnitQuarter, UnitYear:
	default:
		return Expression{}, fmt.Errorf("relative_period has unsupported unit %q", expression.Unit)
	}
	switch expression.Anchor {
	case AnchorCurrentTime, AnchorFirstAvailable, AnchorLastAvailable:
		if expression.AnchorValue != nil {
			return Expression{}, fmt.Errorf("relative_period anchor %q does not accept anchor_value", expression.Anchor)
		}
	case AnchorFixed:
		if expression.AnchorValue == nil {
			return Expression{}, fmt.Errorf("relative_period fixed anchor requires anchor_value")
		}
		value, err := canonicalValue(*expression.AnchorValue, valueKind)
		if err != nil {
			return Expression{}, fmt.Errorf("relative_period fixed anchor: %w", err)
		}
		expression.AnchorValue = &value
	default:
		return Expression{}, fmt.Errorf("relative_period has unsupported anchor %q", expression.Anchor)
	}
	return Expression{
		Kind: expression.Kind, Direction: expression.Direction, Count: expression.Count,
		Unit: expression.Unit, IncludeCurrent: expression.IncludeCurrent,
		Anchor: expression.Anchor, AnchorValue: expression.AnchorValue,
	}, nil
}

func canonicalValues(values []Value, kind ValueKind) ([]Value, error) {
	unique := make(map[string]Value, len(values))
	for _, value := range values {
		canonical, err := canonicalValue(value, kind)
		if err != nil {
			return nil, err
		}
		keyBytes, _ := json.Marshal(canonical)
		unique[string(keyBytes)] = canonical
	}
	result := make([]Value, 0, len(unique))
	for _, value := range unique {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return compareValues(result[i], result[j]) < 0 })
	return result, nil
}

func canonicalValue(value Value, want ValueKind) (Value, error) {
	if value.Kind != want {
		return Value{}, fmt.Errorf("filter value kind %q does not match %q", value.Kind, want)
	}
	switch want {
	case ValueString:
		text, ok := value.Value.(string)
		if !ok {
			return Value{}, fmt.Errorf("string filter value must be a string")
		}
		return Value{Kind: want, Value: text}, nil
	case ValueBoolean:
		boolean, ok := value.Value.(bool)
		if !ok {
			return Value{}, fmt.Errorf("boolean filter value must be a boolean")
		}
		return Value{Kind: want, Value: boolean}, nil
	case ValueInteger:
		text, ok := value.Value.(string)
		if !ok {
			return Value{}, fmt.Errorf("integer filter value must use a precision-safe string")
		}
		integer := new(big.Int)
		if _, ok := integer.SetString(text, 10); !ok {
			return Value{}, fmt.Errorf("invalid integer filter value %q", text)
		}
		return Value{Kind: want, Value: integer.String()}, nil
	case ValueDecimal:
		text, ok := value.Value.(string)
		if !ok || !decimalPattern.MatchString(text) {
			return Value{}, fmt.Errorf("invalid decimal filter value %q", text)
		}
		return Value{Kind: want, Value: canonicalDecimal(text)}, nil
	case ValueDate:
		text, ok := value.Value.(string)
		if !ok {
			return Value{}, fmt.Errorf("date filter value must be a string")
		}
		date, err := time.Parse("2006-01-02", text)
		if err != nil {
			return Value{}, fmt.Errorf("invalid date filter value %q", text)
		}
		return Value{Kind: want, Value: date.Format("2006-01-02")}, nil
	case ValueTimestamp:
		text, ok := value.Value.(string)
		if !ok {
			return Value{}, fmt.Errorf("timestamp filter value must be a string")
		}
		timestamp, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return Value{}, fmt.Errorf("invalid timestamp filter value %q", text)
		}
		return Value{Kind: want, Value: timestamp.UTC().Format(time.RFC3339Nano)}, nil
	default:
		return Value{}, fmt.Errorf("unsupported filter value kind %q", want)
	}
}

func canonicalDecimal(value string) string {
	sign := ""
	if strings.HasPrefix(value, "-") {
		sign, value = "-", value[1:]
	} else {
		value = strings.TrimPrefix(value, "+")
	}
	whole, fraction, hasFraction := strings.Cut(value, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	if hasFraction {
		fraction = strings.TrimRight(fraction, "0")
	}
	if whole == "0" && fraction == "" {
		sign = ""
	}
	if fraction == "" {
		return sign + whole
	}
	return sign + whole + "." + fraction
}

func compareValues(left, right Value) int {
	switch left.Kind {
	case ValueBoolean:
		return strings.Compare(strconv.FormatBool(left.Value.(bool)), strconv.FormatBool(right.Value.(bool)))
	case ValueInteger:
		a, b := new(big.Int), new(big.Int)
		a.SetString(left.Value.(string), 10)
		b.SetString(right.Value.(string), 10)
		return a.Cmp(b)
	case ValueDecimal:
		a, b := new(big.Rat), new(big.Rat)
		a.SetString(left.Value.(string))
		b.SetString(right.Value.(string))
		return a.Cmp(b)
	default:
		return strings.Compare(fmt.Sprint(left.Value), fmt.Sprint(right.Value))
	}
}

func comparisonOperatorAllowed(operator Operator, kind ValueKind) bool {
	switch operator {
	case OperatorEquals, OperatorNotEquals:
		return true
	case OperatorContains, OperatorNotContains, OperatorStartsWith, OperatorEndsWith:
		return kind == ValueString
	case OperatorGreaterThan, OperatorGreaterThanOrEqual, OperatorLessThan, OperatorLessThanOrEqual:
		return kind != ValueBoolean
	default:
		return false
	}
}

func validValueKind(kind ValueKind) bool {
	switch kind {
	case ValueString, ValueBoolean, ValueInteger, ValueDecimal, ValueDate, ValueTimestamp:
		return true
	default:
		return false
	}
}

func BindingKey(dashboardID string, scope Scope, pageID, bindingID string) string {
	identity := dashboardID + "\x00" + string(scope) + "\x00" + pageID + "\x00" + bindingID
	sum := sha256.Sum256([]byte(identity))
	return "fb_" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
}

func EncodeTypedV1(expression Expression, valueKind ValueKind) (string, error) {
	canonical, err := Canonicalize(expression, valueKind)
	if err != nil {
		return "", err
	}
	bytes, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func DecodeTypedV1(encoded string, valueKind ValueKind) (Expression, error) {
	bytes, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Expression{}, fmt.Errorf("decode typed_v1 filter: %w", err)
	}
	var expression Expression
	decoder := json.NewDecoder(strings.NewReader(string(bytes)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&expression); err != nil {
		return Expression{}, fmt.Errorf("decode typed_v1 filter expression: %w", err)
	}
	return Canonicalize(expression, valueKind)
}
