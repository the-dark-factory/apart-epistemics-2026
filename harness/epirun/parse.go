package main

// This file copies, by logic and not by import, the return-parsing rules of
// tools/tcb/parse.go (RulesCurrent): the benchmark's own way of reading a
// generated Form 1040 reply.

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// scoredLabels are the 19 Form 1040 line labels the benchmark scores, in
// order, as in tools/tcb/lines.go.
var scoredLabels = []string{
	"Line 1a: Total amount from Form(s) W-2, box 1",
	"Line 9: Add lines 1z, 2b, 3b, 4b, 5b, 6b, 7, and 8. This is your total income",
	"Line 10: Adjustments to income from Schedule 1, line 26",
	"Line 11: Subtract line 10 from line 9. This is your adjusted gross income",
	"Line 12: Standard deduction or itemized deductions (from Schedule A)",
	"Line 15: Subtract line 14 from line 11. If zero or less, enter -0-. This is your taxable income",
	"Line 16: Tax",
	"Line 19: Child tax credit or credit for other dependents from Schedule 8812",
	"Line 24: Add lines 22 and 23. This is your total tax",
	"Line 25d: Add lines 25a through 25c",
	"Line 26: 2024 estimated tax payments and amount applied from 2023 return",
	"Line 27: Earned income credit (EIC)",
	"Line 28: Additional child tax credit from Schedule 8812",
	"Line 29: American opportunity credit from Form 8863, line 8",
	"Line 32: Add lines 27, 28, 29, and 31. These are your total other payments and refundable credits",
	"Line 33: Add lines 25d, 26, and 32. These are your total payments",
	"Line 34: If line 33 is more than line 24, subtract line 24 from line 33. This is the amount you overpaid",
	"Line 35a: Amount of line 34 you want refunded to you.",
	"Line 37: Subtract line 33 from line 24. This is the amount you owe",
}

// labelPrefix returns the label up to its first colon ("Line 16").
func labelPrefix(label string) string {
	p, _, _ := strings.Cut(label, ":")
	return p
}

// pyIsSpace reports whether r is whitespace by Python's str.isspace.
func pyIsSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// pyStrip mirrors Python's str.strip() with no argument.
func pyStrip(s string) string {
	return strings.TrimFunc(s, pyIsSpace)
}

// parseMoneyAmount mirrors TaxReturnEvaluator.parse_money_amount: blank is 0,
// every "$" and "," is dropped, "(x)" becomes "-x", Python float(); anything
// float() rejects is 0.
func parseMoneyAmount(s string) float64 {
	if pyStrip(s) == "" {
		return 0
	}
	cleaned := strings.ReplaceAll(s, "$", "")
	cleaned = strings.ReplaceAll(cleaned, ",", "")
	cleaned = pyStrip(cleaned)
	if strings.HasPrefix(cleaned, "(") && strings.HasSuffix(cleaned, ")") && len(cleaned) >= 2 {
		cleaned = "-" + cleaned[1:len(cleaned)-1]
	}
	v, err := pyFloat(cleaned)
	if err != nil {
		return 0
	}
	return v
}

// errPyFloat is returned when a string is not accepted by Python's float().
var errPyFloat = errors.New("not a Python float literal")

// pyFloat mirrors Python's float(str).
func pyFloat(s string) (float64, error) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		case pyIsSpace(r):
			b.WriteByte(' ')
		case unicode.Is(unicode.Nd, r):
			b.WriteByte(byte('0' + ndValue(r)))
		default:
			b.WriteRune(r)
		}
	}
	t := pyStrip(b.String())
	if !validPyFloat(t) {
		return 0, errPyFloat
	}
	lower := strings.ToLower(t)
	body := strings.TrimLeft(lower, "+-")
	neg := strings.HasPrefix(lower, "-")
	switch body {
	case "inf", "infinity":
		if neg {
			return math.Inf(-1), nil
		}
		return math.Inf(1), nil
	case "nan":
		return math.NaN(), nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(t, "_", ""), 64)
	if err != nil {
		var ne *strconv.NumError
		if errors.As(err, &ne) && errors.Is(ne.Err, strconv.ErrRange) {
			return v, nil
		}
		return 0, errors.Join(errPyFloat, err)
	}
	return v, nil
}

// ndValue returns the decimal value of a Unicode Nd digit.
func ndValue(r rune) int {
	n := 0
	for unicode.Is(unicode.Nd, r-1) {
		r--
		n++
	}
	return n % 10
}

// validPyFloat checks the ASCII grammar of Python's float() after stripping.
func validPyFloat(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	switch strings.ToLower(s[i:]) {
	case "inf", "infinity", "nan":
		return true
	}
	digits := func() int {
		start := i
		if i >= len(s) || !isDigit(s[i]) {
			return 0
		}
		i++
		for i < len(s) {
			if isDigit(s[i]) {
				i++
				continue
			}
			if s[i] == '_' && i+1 < len(s) && isDigit(s[i+1]) {
				i += 2
				continue
			}
			break
		}
		return i - start
	}
	intPart := digits()
	fracPart := 0
	if i < len(s) && s[i] == '.' {
		i++
		fracPart = digits()
	}
	if intPart == 0 && fracPart == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if digits() == 0 {
			return false
		}
	}
	return i == len(s)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// generatedFragment returns generated.split(label)[1].split("\n")[0].
func generatedFragment(generated, label string) (frag string, found bool) {
	idx := strings.Index(generated, label)
	if idx < 0 {
		return "", false
	}
	rest := generated[idx+len(label):]
	if j := strings.Index(rest, label); j >= 0 {
		rest = rest[:j]
	}
	if j := strings.IndexByte(rest, '\n'); j >= 0 {
		rest = rest[:j]
	}
	return rest, true
}

// parseGeneratedValue mirrors TaxReturnEvaluator.parse_generated_value. The
// second result reports whether the label was found with a "|" on its line;
// a missing label or "|" gives 0 and false, as in the benchmark.
func parseGeneratedValue(generated, label string) (float64, bool) {
	frag, found := generatedFragment(generated, label)
	if !found || !strings.Contains(frag, "|") {
		return 0, false
	}
	return parseMoneyAmount(pyStrip(frag[strings.LastIndex(frag, "|")+1:])), true
}
