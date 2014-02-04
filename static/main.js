(function() {
  var addLimit, dedentMetric, doQuery, fieldToParser, flash, parseComparison, parseDuration, parseGroupLimit, parseIncludeEmptyTargets, parseLimit, parseMetric, parseUrl, populateField, populateFieldsFromUrl, populateGroupLimits, populateLimits, removeLimit, setGroupLimitFieldVisibility, spinjsSettings, writeUrlFromFields;

  spinjsSettings = {
    lines: 13,
    length: 15,
    width: 8,
    radius: 26,
    corners: 1,
    rotate: 15,
    direction: 1,
    color: '#4551c2',
    speed: 0.9,
    trail: 100,
    shadow: false,
    hwaccel: false,
    className: 'spinner',
    zIndex: 2e9,
    top: 'auto',
    left: 'auto'
  };

  parseUrl = function(url) {
    var components, idx, name, parser, part, raw, rawValue, required, _i, _len, _ref, _ref1;
    if (!url) {
      return null;
    }
    url = url.replace(/^(\/?check\?)?(.*)$/, "$2");
    if (url === "") {
      return null;
    }
    raw = {};
    _ref = url.split("&");
    for (_i = 0, _len = _ref.length; _i < _len; _i++) {
      part = _ref[_i];
      idx = part.indexOf("=");
      raw[part.slice(0, idx)] = decodeURIComponent(part.slice(idx + 1));
    }
    components = {};
    for (name in fieldToParser) {
      _ref1 = fieldToParser[name], parser = _ref1[0], required = _ref1[1];
      rawValue = raw[name];
      if (required) {
        if (!rawValue) {
          throw "Missing field: " + name;
        }
      }
      components[name] = parser(rawValue);
    }
    return components;
  };

  parseMetric = function(raw) {
    return raw;
  };

  parseLimit = function(raw) {
    var comp, limit, limits, name, value, _i, _len, _ref, _ref1;
    limits = [];
    _ref = raw.split(",");
    for (_i = 0, _len = _ref.length; _i < _len; _i++) {
      limit = _ref[_i];
      _ref1 = parseComparison(limit), name = _ref1[0], comp = _ref1[1], value = _ref1[2];
      if (name !== "avg" && name !== "min" && name !== "max" && name !== "sum") {
        throw "Bad comparison for limit: " + name;
      }
      limits.push([name, comp, value]);
    }
    return limits;
  };

  parseGroupLimit = function(raw) {
    var comp, name, value, _ref;
    if (!raw) {
      return null;
    }
    if (raw === "all" || raw === "any") {
      return raw;
    }
    if (raw.match(/,/)) {
      throw "Bad group limit (only one comparison allowed): " + raw;
    }
    _ref = parseComparison(raw), name = _ref[0], comp = _ref[1], value = _ref[2];
    if (name !== "count" && name !== "fraction") {
      throw "Bad comparison for group limit: " + name;
    }
    return [name, comp, value];
  };

  parseDuration = function(raw) {
    if (!raw.match(/^(\d+[dhms])+$/)) {
      throw "Bad duration: " + raw;
    }
    return raw;
  };

  parseIncludeEmptyTargets = function(raw) {
    return raw === "true";
  };

  parseComparison = function(raw) {
    var comp, name, parts, value, _;
    parts = raw.match(/^([^<=>]+)(<|<=|=|>=|>)([\d\.]+)$/);
    if (!parts) {
      throw "Bad comparison: " + raw;
    }
    _ = parts[0], name = parts[1], comp = parts[2], value = parts[3];
    value = Number(value);
    if ((value == null) || isNaN(value)) {
      throw "Bad number in comparison: " + parts[3];
    }
    return [name, comp, value];
  };

  fieldToParser = {
    metric: [parseMetric, true],
    from: [parseDuration, true],
    until: [parseDuration, true],
    limit: [parseLimit, true],
    group_limit: [parseGroupLimit, false],
    include_empty_targets: [parseIncludeEmptyTargets, false]
  };

  populateFieldsFromUrl = function() {
    var err, parsed;
    try {
      parsed = parseUrl($("#url-box").val());
    } catch (_error) {
      err = _error;
      $("#url .error").text(err).show();
      $("#url-box").addClass("bad");
      return;
    }
    $("#url-box").removeClass("bad");
    $("#url .error").hide();
    if (!parsed) {
      return;
    }
    populateField($("#metric-box"), indentMetric(parsed.metric));
    populateField($("#from-box"), parsed.from);
    populateField($("#until-box"), parsed.until);
    $("#limits .limit").remove();
    populateLimits(parsed.limit);
    populateGroupLimits(parsed.group_limit);
    return $("#include-empty-targets-checkbox").prop("checked", parsed.include_empty_targets);
  };

  window.indentMetric = function(m) {
    var addCurrent, addIndent, cur, indent, output, start;
    output = "";
    cur = 0;
    start = 0;
    indent = 0;
    addIndent = function(amount) {
      var i, _i, _results;
      _results = [];
      for (i = _i = 0; 0 <= amount ? _i < amount : _i > amount; i = 0 <= amount ? ++_i : --_i) {
        _results.push(output += "  ");
      }
      return _results;
    };
    addCurrent = function() {
      output += m.slice(start, cur);
      return start = cur;
    };
    while (cur < m.length) {
      switch (m[cur]) {
        case "(":
          cur += 1;
          indent += 1;
          addCurrent();
          output += "\n";
          addIndent(indent);
          break;
        case ")":
          addCurrent();
          output += "\n";
          indent -= 1;
          addIndent(indent);
          cur += 1;
          addCurrent();
          break;
        case ",":
          cur += 1;
          addCurrent();
          output += "\n";
          addIndent(indent);
          break;
        default:
          cur += 1;
      }
    }
    addCurrent();
    return output;
  };

  populateField = function($field, value) {
    $field.val(value);
    return flash($field);
  };

  populateLimits = function(limits) {
    var $limit, limit, _i, _len, _results;
    _results = [];
    for (_i = 0, _len = limits.length; _i < _len; _i++) {
      limit = limits[_i];
      $limit = addLimit();
      $limit.find("select.limit-name").val(limit[0]);
      $limit.find("select.comparison").val(limit[1]);
      $limit.find("input.value-box").val(limit[2]);
      _results.push(flash($limit.find("select, input")));
    }
    return _results;
  };

  populateGroupLimits = function(groupLimit) {
    var $group;
    $group = $("#group-limits .limit");
    if (!groupLimit) {
      $group.find("select.limit-name").val("none");
      return;
    }
    if (groupLimit === "any" || groupLimit === "all") {
      $group.find("select.limit-name").val(groupLimit);
      flash($group.find("select"));
      return;
    }
    $group.find("select.limit-name").val(groupLimit[0]);
    $group.find("select.comparison").val(groupLimit[1]);
    $group.find("input.value-box").val(groupLimit[2]);
    return flash($group.find("select, input"));
  };

  flash = function($e) {
    $e.addClass("flash");
    return setTimeout((function() {
      return $e.removeClass("flash");
    }), 0);
  };

  addLimit = function() {
    var $newLimit;
    $newLimit = $("#limit-template").clone();
    $newLimit.removeAttr("id");
    $newLimit.insertBefore($("#add-limit").parent());
    return $newLimit;
  };

  removeLimit = function(e) {
    $(e.target).parent().remove();
    return writeUrlFromFields();
  };

  setGroupLimitFieldVisibility = function() {
    var $groupLimit, name;
    $groupLimit = $("#group-limits .limit");
    name = $groupLimit.find("select.limit-name").val();
    if (name === "none" || name === "any" || name === "all") {
      $groupLimit.find("select.comparison").hide();
      return $groupLimit.find("input.value-box").hide();
    } else {
      $groupLimit.find("select.comparison").show();
      return $groupLimit.find("input.value-box").show();
    }
  };

  writeUrlFromFields = function() {
    var $groupLimit, $l, $url, comp, from, groupLimitType, l, limit, limits, metric, name, query, untilVal, val, _i, _len, _ref;
    metric = encodeURIComponent(dedentMetric($("#metric-box").val()));
    from = encodeURIComponent($("#from-box").val());
    untilVal = encodeURIComponent($("#until-box").val());
    limits = [];
    _ref = $("#limits .limit");
    for (_i = 0, _len = _ref.length; _i < _len; _i++) {
      l = _ref[_i];
      $l = $(l);
      name = $l.find("select.limit-name").val();
      comp = $l.find("select.comparison").val();
      val = $l.find("input.value-box").val();
      limits.push(name + comp + val);
    }
    limit = encodeURIComponent(limits.join(","));
    query = "/check?metric=" + metric + "&from=" + from + "&until=" + untilVal + "&limit=" + limit;
    $groupLimit = $("#group-limits .limit");
    groupLimitType = $groupLimit.find("select.limit-name").val();
    if (groupLimitType !== "none") {
      if (groupLimitType === "all" || groupLimitType === "any") {
        query += "&group_limit=" + groupLimitType;
      } else {
        name = $groupLimit.find("select.limit-name").val();
        comp = $groupLimit.find("select.comparison").val();
        val = $groupLimit.find("input.value-box").val();
        limit = encodeURIComponent(name + comp + val);
        query += "&group_limit=" + limit;
      }
    }
    if ($("#include-empty-targets-checkbox").is(":checked")) {
      query += "&include_empty_targets=true";
    }
    $url = $("#url-box");
    $url.val(query);
    return flash($url);
  };

  dedentMetric = function(m) {
    return m.replace(/\s/g, "");
  };

  doQuery = function() {
    var spinner, url;
    $("#query-result").hide();
    url = $("#url-box").val();
    if (!url.match(/^\/check?/)) {
      $("#query-result").removeClass("good").addClass("bad").text("Bad query string.").show();
      resizeQueryResult();
      return;
    }
    spinner = new Spinner(spinjsSettings).spin($("#query-wrapper")[0]);
    return $.ajax({
      cache: false,
      error: function(jqXHR) {
        return $("#query-result").removeClass("good").addClass("bad").text(jqXHR.responseText);
      },
      success: function(data, _, jqXHR) {
        return $("#query-result").removeClass("bad").addClass("good").text(data);
      },
      type: "GET",
      url: url,
      complete: function() {
        spinner.stop();
        $("#query-result").show();
        return resizeQueryResult();
      }
    });
  };

  window.resizeQueryResult = function() {
    var scrollHeight;
    $("#query-result").height("0px");
    scrollHeight = $("#query-result")[0].scrollHeight;
    return $("#query-result").height(Math.max(100, scrollHeight));
  };

  $(function() {
    setGroupLimitFieldVisibility();
    $("#url-box").on("input", populateFieldsFromUrl);
    $("#breakdown").on("input", "textarea,input", writeUrlFromFields);
    $("#breakdown").on("change", "select", writeUrlFromFields);
    $("#breakdown").on("click", "label", writeUrlFromFields);
    $("#add-limit").on("click", addLimit);
    $("#limits").on("click", ".remove-limit", removeLimit);
    $("#group-limits select.limit-name").on("change", setGroupLimitFieldVisibility);
    return $("#query").on("click", doQuery);
  });

}).call(this);
