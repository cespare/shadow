package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

/*
check?metric=web.requests.rate&range=5m&limit=avg>300,avg<=1e3&group_limit=count>3
check?metric=web.requests.rate&range=5m&limits=avg>300,avg<=1e3&group_limit=any
*/

func init() {
	for k, v := range ParamToAggMethod {
		aggMethodToParam[v] = k
	}
	for k, v := range ParamToComparison {
		comparisonToParam[v] = k
	}
	comparisonToParam[CompEq] = "="
	for k, v := range ParamToGroupAggMethod {
		groupAggMethodToParam[v] = k
	}
}

type AggMethod int

const (
	AggAvg AggMethod = iota
	AggMin
	AggMax
	AggSum
)

var ParamToAggMethod = map[string]AggMethod{
	"avg": AggAvg,
	"min": AggMin,
	"max": AggMax,
	"sum": AggSum,
}

var aggMethodToParam = make(map[AggMethod]string)

func (a AggMethod) String() string {
	return aggMethodToParam[a]
}

type Comparison int

const (
	CompEq Comparison = iota
	CompLt
	CompLeq
	CompGt
	CompGeq
)

var ParamToComparison = map[string]Comparison{
	"=":  CompEq,
	"==": CompEq,
	"<":  CompLt,
	"<=": CompLeq,
	">":  CompGt,
	">=": CompGeq,
}

var comparisonToParam = make(map[Comparison]string)

func (c Comparison) String() string {
	return comparisonToParam[c]
}

var ComparisonInverse = map[Comparison]Comparison{
	CompEq:  CompEq,
	CompLt:  CompGt,
	CompLeq: CompGeq,
	CompGt:  CompLt,
	CompGeq: CompLeq,
}

var limitRegex = regexp.MustCompile(`^([^<=>]+)([<=>]+)(.+)$`)

type Limit struct {
	Comparison
	Bound float64
}

func (l Limit) Within(n float64) bool {
	var withinLimit bool
	switch l.Comparison {
	case CompLt:
		withinLimit = n < l.Bound
	case CompLeq:
		withinLimit = n <= l.Bound
	case CompEq:
		withinLimit = n == l.Bound
	case CompGeq:
		withinLimit = n >= l.Bound
	case CompGt:
		withinLimit = n > l.Bound
	}
	return withinLimit
}

func (l Limit) String() string {
	return fmt.Sprintf("%s%f", l.Comparison, l.Bound)
}

// e.g.: parts = ["avg", "<", "3.5"] or ["4e9", ">=", "max"]
func ParseLimit(parts []string) (limit Limit, aggMethod string, err error) {
	comp, ok := ParamToComparison[parts[1]]
	if !ok {
		return limit, "", fmt.Errorf("unknown comparison: %s", parts[1])
	}
	aggMethodParam, boundParam := parts[0], parts[2]
	bound, err := strconv.ParseFloat(boundParam, 64)
	// Switch if necessary
	if err != nil {
		aggMethodParam, boundParam = boundParam, aggMethodParam
		comp = ComparisonInverse[comp]
		bound, err = strconv.ParseFloat(boundParam, 64)
		if err != nil {
			return limit, "", fmt.Errorf("cannot parse bound %s: %s", boundParam, err)
		}
	}
	limit.Comparison = comp
	limit.Bound = bound
	return limit, aggMethodParam, nil
}

type GroupAggMethod int

const (
	GroupAggCount GroupAggMethod = iota
	GroupAggFraction
)

var ParamToGroupAggMethod = map[string]GroupAggMethod{
	"count":    GroupAggCount,
	"fraction": GroupAggFraction,
}

var groupAggMethodToParam = make(map[GroupAggMethod]string)

func (a GroupAggMethod) String() string {
	return groupAggMethodToParam[a]
}

type IndividualLimit struct {
	Limit
	AggMethod
}

func (l *IndividualLimit) String() string {
	return fmt.Sprintf("%s%s", l.AggMethod, l.Limit)
}

func ParseIndividualLimits(s string) ([]*IndividualLimit, error) {
	limits := []*IndividualLimit{}
	for _, limit := range strings.Split(s, ",") {
		limit = strings.TrimSpace(limit)
		if limit == "" {
			continue
		}
		parts := limitRegex.FindAllStringSubmatch(limit, -1)
		if len(parts) != 1 || len(parts[0]) != 4 {
			return nil, fmt.Errorf("invalid limit: '%s'", limit)
		}
		limit, aggMethodParam, err := ParseLimit(parts[0][1:])
		if err != nil {
			return nil, err
		}
		aggMethod, ok := ParamToAggMethod[aggMethodParam]
		if !ok {
			return nil, fmt.Errorf("unknown aggregation method: '%s'", aggMethodParam)
		}
		limits = append(limits, &IndividualLimit{
			Limit:     limit,
			AggMethod: aggMethod,
		})
	}
	if len(limits) == 0 {
		return nil, fmt.Errorf("no limits provided")
	}
	return limits, nil
}

type GroupLimit struct {
	Limit
	GroupAggMethod
}

var GroupLimitAny = &GroupLimit{
	Limit{CompGt, 0.0},
	GroupAggCount,
}

var GroupLimitAll = &GroupLimit{
	Limit{CompEq, 1.0},
	GroupAggFraction,
}

var GroupLimitShorthands = map[string]*GroupLimit{
	"any": GroupLimitAny,
	"all": GroupLimitAll,
}

func (l *GroupLimit) String() string {
	return fmt.Sprintf("%s%s", l.GroupAggMethod, l.Limit)
}

func ParseGroupLimits(s string) ([]*GroupLimit, error) {
	if limit, ok := GroupLimitShorthands[s]; ok {
		return []*GroupLimit{limit}, nil
	}

	limits := []*GroupLimit{}
	for _, limit := range strings.Split(s, ",") {
		limit = strings.TrimSpace(limit)
		if limit == "" {
			continue
		}
		parts := limitRegex.FindAllStringSubmatch(limit, -1)
		if len(parts) != 1 || len(parts[0]) != 4 {
			return nil, fmt.Errorf("invalid group_limit: '%s'", limit)
		}
		limit, aggMethodParam, err := ParseLimit(parts[0][1:])
		if err != nil {
			return nil, err
		}
		aggMethod, ok := ParamToGroupAggMethod[aggMethodParam]
		if !ok {
			return nil, fmt.Errorf("unknown group aggregation limit type: '%s'", aggMethodParam)
		}
		if limit.Bound < 0 || (aggMethod == GroupAggFraction && limit.Bound > 1) {
			return nil, fmt.Errorf("bad group aggregation bound: %v", limit.Bound)
		}
		limits = append(limits, &GroupLimit{
			Limit:          limit,
			GroupAggMethod: aggMethod,
		})
	}
	return limits, nil
}

type Check struct {
	Metric              string // Graphite metric name
	Range               time.Duration
	Until               time.Duration
	IncludeEmptyTargets bool
	IndividualLimits    []*IndividualLimit
	GroupLimits         []*GroupLimit
}

func getSingleParam(q url.Values, name string) (string, error) {
	values, ok := q[name]
	if !ok {
		return "", fmt.Errorf("missing parameter: %s", name)
	}
	if len(values) > 1 {
		return "", fmt.Errorf("parameter %s supplied more than once", name)
	}
	return values[0], nil
}

func ParseCheckURL(u *url.URL) (*Check, error) {
	q := u.Query()
	metric, err := getSingleParam(q, "metric")
	if err != nil {
		return nil, err
	}
	rng, err := getSingleParam(q, "range")
	if err != nil {
		return nil, err
	}
	until, err := getSingleParam(q, "until")
	if err != nil {
		return nil, err
	}
	rngDuration, err := time.ParseDuration(rng)
	if err != nil {
		return nil, fmt.Errorf("could not parse range: %s", err)
	}
	untilDuration, err := time.ParseDuration(until)
	if err != nil {
		return nil, fmt.Errorf("could not parse until: %s", err)
	}
	limitString, err := getSingleParam(q, "limit")
	if err != nil {
		return nil, err
	}
	individualLimits, err := ParseIndividualLimits(limitString)
	if err != nil {
		return nil, fmt.Errorf("could not parse limit: %s", err)
	}
	var groupLimits []*GroupLimit
	groupLimitParam, ok := q["group_limit"]
	if ok {
		if len(groupLimitParam) > 1 {
			return nil, fmt.Errorf("parameter group_limit supplied more than once")
		}
		groupLimits, err = ParseGroupLimits(groupLimitParam[0])
		if err != nil {
			return nil, fmt.Errorf("could not parse group_limit: %s", err)
		}
	}

	c := &Check{
		Metric:              metric,
		Range:               rngDuration,
		Until:               untilDuration,
		IncludeEmptyTargets: q.Get("include_empty_targets") == "true",
		IndividualLimits:    individualLimits,
		GroupLimits:         groupLimits,
	}
	return c, nil
}

// Note it's not documented that url.Values.Encode() emits in key-sorted order (although it does), so this
// could break in future...
func (c *Check) String() string {
	individualLimits := []string{}
	for _, limit := range c.IndividualLimits {
		individualLimits = append(individualLimits, limit.String())
	}
	sort.Strings(individualLimits)
	groupLimits := []string{}
	for _, limit := range c.GroupLimits {
		groupLimits = append(groupLimits, limit.String())
	}
	sort.Strings(groupLimits)
	v := url.Values{
		"metric":      {c.Metric},
		"range":       {c.Range.String()},
		"until":       {c.Until.String()},
		"limit":       {strings.Join(individualLimits, ",")},
		"group_limit": {strings.Join(groupLimits, ",")},
	}
	return v.Encode()
}

func (c *Check) MakeGraphiteURL() string {
	values := url.Values{
		"target": {c.Metric},
		"format": {"json"},
		"from":   {fmt.Sprintf("-%ds", int(c.Range.Seconds()))},
		"until":  {fmt.Sprintf("-%ds", int(c.Until.Seconds()))},
	}
	return GraphiteURL("render?" + values.Encode())
}

// TODO: The user could configure the Render chart URL in the toml (adjust time range, size, etc.)
func (c *Check) MakeGraphiteRenderURL() string {
	minutes := 30
	title := fmt.Sprintf("Last %d minutes of data for %s", minutes, c.Metric)
	values := url.Values{
		"target": {c.Metric},
		"from":   {fmt.Sprintf("-%dmins", minutes)},
		"width":  {"800"},
		"height": {"600"},
		"yMin":   {"0"},
		"title":  {title},
	}
	return GraphiteURL("render?" + values.Encode())
}

// TODO: retries
func (c *Check) DoCheck(w http.ResponseWriter) {
	s := &Status{}
	defer func() {
		s.WriteHTTPResponse(w)
	}()
	resp, err := client.Get(c.MakeGraphiteURL())
	if err != nil {
		s.Code = http.StatusBadGateway
		s.Message = "Error contacting the Graphite server: " + err.Error()
		return
	}
	defer resp.Body.Close()
	s.Code = resp.StatusCode
	if resp.Header.Get("Content-Type") != "application/json" {
		s.Message = "Non-JSON response from Graphite (exception?)"
		return
	}
	decoder := json.NewDecoder(resp.Body)
	result := []*GraphiteResult{}
	if err := decoder.Decode(&result); err != nil {
		s.Code = http.StatusBadGateway
		s.Message = "Could not read JSON response from Graphite: " + err.Error()
		return
	}
	ok, reason := CompareGraphiteResultWithCheck(result, c)
	if ok {
		s.Code = http.StatusOK
		s.Message = ""
		return
	}
	s.Code = http.StatusInternalServerError
	s.Message = reason
}

func HandleGraphiteChecks(w http.ResponseWriter, r *http.Request) {
	check, err := ParseCheckURL(r.URL)
	if err != nil {
		http.Error(w, "Invalid check: "+err.Error(), http.StatusBadRequest)
		return
	}
	check.DoCheck(w)
}

type GraphitePoint struct {
	Null  bool
	Value float64
}

func (p *GraphitePoint) UnmarshalJSON(b []byte) error {
	// Easy (hacky) way to get at the numbers -- I don't actually care about the timestamp, so it's fine for it
	// to be a float64 (I'm just going to throw it away).
	var values [2]*float64
	if err := json.Unmarshal(b, &values); err != nil {
		return err
	}
	if values[0] == nil {
		p.Null = true
		p.Value = 0
		return nil
	}
	p.Null = false
	p.Value = *values[0]
	return nil
}

type GraphiteResult struct {
	Target     string
	Datapoints []*GraphitePoint
}

type CheckResult struct {
	OK     bool
	Ignore bool
	Reason string
}

func CompareGraphiteResultWithCheck(result []*GraphiteResult, c *Check) (ok bool, reason string) {
	if len(result) == 0 {
		return false, "No datapoints returned from Graphite."
	}
	if len(c.GroupLimits) > 0 {
		checkResults := make([]CheckResult, 0, len(result))
		for _, r := range result {
			checkResult := r.compareWithCheck(c)
			if checkResult.Ignore {
				continue
			}
			checkResults = append(checkResults, checkResult)
		}
		return checkGroupResults(checkResults, c)
	}
	if len(result) > 1 {
		return false, "group_limit not given, yet Graphite returned multiple datapoints"
	}
	checkResult := result[0].compareWithCheck(c)
	reason = checkResult.Reason
	if !checkResult.OK {
		reason += "\nChart2: " + c.MakeGraphiteRenderURL()
	}
	return checkResult.OK, reason
}

func checkGroupResults(results []CheckResult, c *Check) (ok bool, reason string) {
	if len(results) == 0 {
		return false, "no data to check"
	}
	var goodCount float64
	for _, r := range results {
		if r.OK {
			goodCount += 1
		}
	}
	goodFrac := goodCount / float64(len(results))
	for _, limit := range c.GroupLimits {
		var ok bool
		switch limit.GroupAggMethod {
		case GroupAggCount:
			ok = limit.Within(goodCount)
		case GroupAggFraction:
			ok = limit.Within(goodFrac)
		}
		if !ok {
			buf := &bytes.Buffer{}
			fmt.Fprintf(buf, "group_limit %s check failed (%v/%v good datapoints)\n", limit, goodCount,
				len(results))
			fmt.Fprintf(buf, "Failed datapoints:\n")
			for _, r := range results {
				if !r.OK {
					fmt.Fprintf(buf, "%s\n", r.Reason)
				}
			}
			fmt.Fprintf(buf, "Chart: %s", c.MakeGraphiteRenderURL())
			return false, buf.String()
		}
	}
	return true, ""
}

func (r *GraphiteResult) compareWithCheck(c *Check) CheckResult {
	for _, limit := range c.IndividualLimits {
		nonNullValues := make([]float64, 0, len(r.Datapoints))
		for _, p := range r.Datapoints {
			if !p.Null {
				nonNullValues = append(nonNullValues, p.Value)
			}
		}
		if len(nonNullValues) == 0 {
			return CheckResult{false, !c.IncludeEmptyTargets, fmt.Sprintf("%s: no datapoints", r.Target)}
		}
		agg := computeAggregate(nonNullValues, limit.AggMethod)
		if !limit.Within(agg) {
			return CheckResult{false, false,
				fmt.Sprintf("%s: limit violated: %s (%s=%.4f)", r.Target, limit, limit.AggMethod, agg)}
		}
	}
	return CheckResult{true, false, ""}
}

func computeAggregate(points []float64, method AggMethod) float64 {
	var result float64
	switch method {
	case AggAvg, AggSum:
		for _, p := range points {
			result += p
		}
		if method == AggAvg {
			result /= float64(len(points))
		}
	case AggMax:
		result = points[0]
		for _, p := range points[1:] {
			if p > result {
				result = p
			}
		}
	case AggMin:
		result = points[0]
		for _, p := range points[1:] {
			if p < result {
				result = p
			}
		}
	}
	return result
}
