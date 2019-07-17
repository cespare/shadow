package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func reverseComparison(cmp string) string {
	switch cmp {
	case "<":
		return ">"
	case "<=":
		return ">="
	case "=":
		return "="
	case ">=":
		return "<="
	case ">":
		return "<"
	default:
		panic(fmt.Sprintf("bad comparison %q", cmp))
	}
}

var limitRegex = regexp.MustCompile(`^([^<=>]+)([<=>]+)(.+)$`)

type limit struct {
	cmp   string
	bound float64
}

func (l limit) within(n float64) bool {
	switch l.cmp {
	case "<":
		return n < l.bound
	case "<=":
		return n <= l.bound
	case "=":
		return n == l.bound
	case ">=":
		return n >= l.bound
	case ">":
		return n > l.bound
	default:
		panic(fmt.Sprintf("bad comparison %q", l.cmp))
	}
}

func (l limit) String() string {
	return fmt.Sprintf("%s%f", l.cmp, l.bound)
}

// parseLimits parses tuples like ["avg", "<", "3.5"] or ["4e9", ">=", "max"].
func parseLimit(parts []string) (lim limit, aggMethod string, err error) {
	lim.cmp = parts[1]
	switch lim.cmp {
	case "<", "<=", "=", ">=", ">":
	default:
		return limit{}, "", fmt.Errorf("unknown comparison: %s", parts[1])
	}
	aggMethodParam, boundParam := parts[0], parts[2]
	lim.bound, err = strconv.ParseFloat(boundParam, 64)
	// Switch if necessary.
	if err != nil {
		aggMethodParam, boundParam = boundParam, aggMethodParam
		lim.cmp = reverseComparison(lim.cmp)
		lim.bound, err = strconv.ParseFloat(boundParam, 64)
		if err != nil {
			return limit{}, "", fmt.Errorf("cannot parse bound %s: %s", boundParam, err)
		}
	}
	return lim, aggMethodParam, nil
}

type individualLimit struct {
	lim       limit
	aggMethod string // avg, min, max, or sum
}

func (l *individualLimit) String() string {
	return fmt.Sprintf("%s%s", l.aggMethod, l.lim)
}

func parseIndividualLimits(s string) ([]*individualLimit, error) {
	var lims []*individualLimit
	for _, s0 := range strings.Split(s, ",") {
		s0 = strings.TrimSpace(s0)
		if s0 == "" {
			continue
		}
		parts := limitRegex.FindAllStringSubmatch(s0, -1)
		if len(parts) != 1 || len(parts[0]) != 4 {
			return nil, fmt.Errorf("invalid limit: %q", s0)
		}
		var err error
		var lim individualLimit
		lim.lim, lim.aggMethod, err = parseLimit(parts[0][1:])
		if err != nil {
			return nil, err
		}
		switch lim.aggMethod {
		case "avg", "min", "max", "sum":
		default:
			return nil, fmt.Errorf("unknown aggregation method: %q", lim.aggMethod)
		}
		lims = append(lims, &lim)
	}
	if len(lims) == 0 {
		return nil, errors.New("no limits provided")
	}
	return lims, nil
}

type check struct {
	metric              string // Graphite metric name
	from                time.Duration
	until               time.Duration
	includeEmptyTargets bool
	individualLimits    []*individualLimit
	groupLimit          string // "", "all", or "any"
}

func getParam(q url.Values, name string) (string, error) {
	values, ok := q[name]
	if !ok {
		return "", fmt.Errorf("missing parameter: %s", name)
	}
	if len(values) > 1 {
		return "", fmt.Errorf("parameter %s supplied more than once", name)
	}
	return values[0], nil
}

func parseCheckURL(u *url.URL) (*check, error) {
	q := u.Query()
	metric, err := getParam(q, "metric")
	if err != nil {
		return nil, err
	}
	from, err := getParam(q, "from")
	if err != nil {
		return nil, err
	}
	until, err := getParam(q, "until")
	if err != nil {
		return nil, err
	}
	fromDuration, err := time.ParseDuration(from)
	if err != nil {
		return nil, fmt.Errorf("could not parse from: %s", err)
	}
	untilDuration, err := time.ParseDuration(until)
	if err != nil {
		return nil, fmt.Errorf("could not parse until: %s", err)
	}
	limitString, err := getParam(q, "limit")
	if err != nil {
		return nil, err
	}
	individualLimits, err := parseIndividualLimits(limitString)
	if err != nil {
		return nil, fmt.Errorf("could not parse limit: %s", err)
	}
	var groupLimit string
	if groupLimitParam, ok := q["group_limit"]; ok {
		if len(groupLimitParam) > 1 {
			return nil, fmt.Errorf("parameter group_limit supplied more than once")
		}
		groupLimit = groupLimitParam[0]
		switch groupLimit {
		case "any", "all":
		default:
			return nil, fmt.Errorf("unknown group_limit value %q", groupLimit)
		}
	}

	return &check{
		metric:              metric,
		from:                fromDuration,
		until:               untilDuration,
		includeEmptyTargets: q.Get("include_empty_targets") == "true",
		individualLimits:    individualLimits,
		groupLimit:          groupLimit,
	}, nil
}

func (c *check) String() string {
	var individualLimits []string
	for _, lim := range c.individualLimits {
		individualLimits = append(individualLimits, lim.String())
	}
	sort.Strings(individualLimits)
	v := url.Values{
		"metric": {c.metric},
		"from":   {c.from.String()},
		"until":  {c.until.String()},
		"limit":  {strings.Join(individualLimits, ",")},
	}
	if c.groupLimit != "" {
		v["group_limit"] = []string{c.groupLimit}
	}
	return v.Encode()
}

func (s *shadow) checkDataURL(c *check) string {
	values := url.Values{
		"target": {c.metric},
		"format": {"json"},
		"from":   {fmt.Sprintf("-%ds", int(c.from.Seconds()))},
		"until":  {fmt.Sprintf("-%ds", int(c.until.Seconds()))},
	}
	return s.graphiteURL + "/render?" + values.Encode()
}

func (s *shadow) checkRenderURL(c *check) string {
	minutes := 30
	title := fmt.Sprintf("Last %d minutes of data for %s", minutes, c.metric)
	values := url.Values{
		"target": {c.metric},
		"from":   {fmt.Sprintf("-%dmins", minutes)},
		"width":  {"800"},
		"height": {"600"},
		"yMin":   {"0"},
		"title":  {title},
	}
	return s.graphiteURL + "/render?" + values.Encode()
}

// TODO: retries
func (s *shadow) doCheck(w http.ResponseWriter, c *check) {
	var st status
	defer st.write(w)

	resp, err := s.client.Get(s.checkDataURL(c))
	if err != nil {
		st.code = http.StatusBadGateway
		st.message = "Error contacting the Graphite server: " + err.Error()
		return
	}
	defer resp.Body.Close()
	st.code = resp.StatusCode
	if getContentType(resp.Header) != "application/json" {
		st.message = "Non-JSON response from Graphite (exception?)"
		if body, err := ioutil.ReadAll(resp.Body); err == nil {
			st.message += "\n\n" + string(body)
		}
		return
	}
	var result []*graphiteResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		st.code = http.StatusBadGateway
		st.message = "Could not read JSON response from Graphite: " + err.Error()
		return
	}
	ok, reason := s.compareResult(result, c)
	if ok {
		st.code = 200
		st.message = ""
		return
	}
	st.code = http.StatusInternalServerError
	st.message = reason
}

func getContentType(header http.Header) string {
	ct := header.Get("Content-Type")
	if ct == "" {
		return ""
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	return mt
}

func (s *shadow) handleGraphiteCheck(w http.ResponseWriter, r *http.Request) {
	c, err := parseCheckURL(r.URL)
	if err != nil {
		http.Error(w, "Invalid check: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.doCheck(w, c)
}

type graphitePoint struct {
	null  bool
	value float64
}

func (p *graphitePoint) UnmarshalJSON(b []byte) error {
	// Easy, hacky way to get at the numbers: we don't care about the
	// timestamp, so it's fine for it to be a float64.
	var values [2]*float64
	if err := json.Unmarshal(b, &values); err != nil {
		return err
	}
	if values[0] == nil {
		p.null = true
		p.value = 0
		return nil
	}
	p.null = false
	p.value = *values[0]
	return nil
}

type graphiteResult struct {
	Target     string
	Datapoints []*graphitePoint
}

type checkResult struct {
	ok     bool
	ignore bool
	reason string
}

func (s *shadow) compareResult(result []*graphiteResult, c *check) (ok bool, reason string) {
	if len(result) == 0 {
		return false, "No datapoints returned from Graphite"
	}
	if c.groupLimit != "" {
		checkResults := make([]checkResult, 0, len(result))
		for _, r := range result {
			checkResult := r.compare(c)
			if checkResult.ignore {
				continue
			}
			checkResults = append(checkResults, checkResult)
		}
		return s.compareGroupResults(checkResults, c)
	}
	if len(result) > 1 {
		return false, "group_limit not given, yet Graphite returned multiple datapoints"
	}
	checkResult := result[0].compare(c)
	reason = checkResult.reason
	if !checkResult.ok {
		reason += "\nChart: " + s.checkRenderURL(c)
	}
	return checkResult.ok, reason
}

func (s *shadow) compareGroupResults(results []checkResult, c *check) (ok bool, reason string) {
	if len(results) == 0 {
		return false, "no data to check"
	}
	var numFailed int
	for _, r := range results {
		if !r.ok {
			numFailed++
		}
	}

	var b strings.Builder
	switch c.groupLimit {
	case "any":
		if numFailed < len(results) {
			return true, ""
		}
		fmt.Fprintf(&b, "group_limit=any check failed for all %d datapoints:\n", len(results))
	case "all":
		if numFailed == 0 {
			return true, ""
		}
		fmt.Fprintf(&b, "group_limit=all check failed for %d/%d datapoints:\n", numFailed, len(results))
	}
	for _, r := range results {
		if !r.ok {
			fmt.Fprintln(&b, r.reason)
		}
	}
	fmt.Fprintf(&b, "Chart: %s", s.checkRenderURL(c))
	return false, b.String()
}

func (r *graphiteResult) compare(c *check) checkResult {
	for _, lim := range c.individualLimits {
		var numNonNull int
		var agg float64
		for _, p := range r.Datapoints {
			if p.null {
				continue
			}
			numNonNull++
			switch lim.aggMethod {
			case "avg", "sum":
				agg += p.value
			case "max":
				if numNonNull == 1 || p.value > agg {
					agg = p.value
				}
			case "min":
				if numNonNull == 1 || p.value < agg {
					agg = p.value
				}
			}
		}
		if numNonNull == 0 {
			return checkResult{
				ignore: !c.includeEmptyTargets,
				reason: fmt.Sprintf("%s: no datapoints", r.Target),
			}
		}
		if lim.aggMethod == "avg" {
			agg /= float64(numNonNull)
		}
		if !lim.lim.within(agg) {
			return checkResult{
				reason: fmt.Sprintf("%s: limit is %s; got %s=%g",
					r.Target, lim, lim.aggMethod, agg),
			}
		}
	}
	return checkResult{ok: true}
}
