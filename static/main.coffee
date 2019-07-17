spinjsSettings =
  lines:     13,        # The number of lines to draw
  length:    15,        # The length of each line
  width:     8,         # The line thickness
  radius:    26,        # The radius of the inner circle
  corners:   1,         # Corner roundness (0..1)
  rotate:    15,        # The rotation offset
  direction: 1,         # 1: clockwise, -1: counterclockwise
  color:     '#4551c2', # #rgb or #rrggbb or array of colors
  speed:     0.9,       # Rounds per second
  trail:     100,       # Afterglow percentage
  shadow:    false,     # Whether to render a shadow
  hwaccel:   false,     # Whether to use hardware acceleration
  className: 'spinner', # The CSS class to assign to the spinner
  zIndex:    2e9,       # The z-index (defaults to 2000000000)
  top:       'auto',    # Top position relative to parent in px
  left:      'auto'     # Left position relative to parent in px

parseUrl = (url) ->
  return null unless url
  url = url.replace(/^(\/?check\?)?(.*)$/, "$2")
  return null if url == ""

  raw = {}
  for part in url.split("&")
    idx = part.indexOf("=")
    raw[part.slice(0, idx)] = decodeURIComponent(part.slice(idx+1))

  components = {}
  for name, [parser, required] of fieldToParser
    rawValue = raw[name]
    if required
      throw "Missing field: #{name}" unless rawValue
    components[name] = parser(rawValue)
  components

parseMetric = (raw) -> raw

parseLimit = (raw) ->
  limits = []
  for limit in raw.split(",")
    [name, comp, value] = parseComparison(limit)
    throw "Bad comparison for limit: #{name}" unless name in ["avg", "min", "max", "sum"]
    limits.push([name, comp, value])
  limits

parseGroupLimit = (raw) ->
  return null unless raw
  if raw in ["all", "any"]
    return raw
  throw "Bad group limit: #{raw}"

parseDuration = (raw) ->
  throw "Bad duration: #{raw}" unless raw.match(/^(\d+[dhms])+$/)
  raw

parseIncludeEmptyTargets = (raw) -> raw == "true"

# avg<0.7 -> ["avg", "<", 0.7]
parseComparison = (raw) ->
  parts = raw.match(/^([^<=>]+)(<|<=|=|>=|>)([\d\.]+)$/)
  throw "Bad comparison: #{raw}" unless parts
  [_, name, comp, value] = parts
  value = Number(value)
  throw "Bad number in comparison: #{parts[3]}" if !value? || isNaN(value)
  [name, comp, value]

fieldToParser =
  metric: [parseMetric, true]
  from: [parseDuration, true]
  until: [parseDuration, true]
  limit: [parseLimit, true]
  group_limit: [parseGroupLimit, false]
  include_empty_targets: [parseIncludeEmptyTargets, false]

populateFieldsFromUrl = ->
  try
    parsed = parseUrl($("#url-box").val())
  catch err
    $("#url .error").text(err).show()
    $("#url-box").addClass("bad")
    return
  $("#url-box").removeClass("bad")
  $("#url .error").hide()
  return unless parsed

  populateField($("#metric-box"), indentMetric(parsed.metric))
  populateField($("#from-box"), parsed.from)
  populateField($("#until-box"), parsed.until)
  $("#limits .limit").remove()
  populateLimits(parsed.limit)
  populateGroupLimits(parsed.group_limit)
  $("#include-empty-targets-checkbox").prop("checked", parsed.include_empty_targets)

window.indentMetric = (m) ->
  output = ""
  cur = 0
  start = 0
  indent = 0

  addIndent = (amount) ->
    for i in [0...amount]
      output += "  "

  addCurrent = ->
    output += m.slice(start, cur)
    start = cur

  while cur < m.length
    switch m[cur]
      when "("
        cur += 1
        indent += 1
        addCurrent()
        output += "\n"
        addIndent(indent)
      when ")"
        addCurrent()
        output += "\n"
        indent -= 1
        addIndent(indent)
        cur += 1
        addCurrent()
      when ","
        cur += 1
        addCurrent()
        output += "\n"
        addIndent(indent)
      else
        cur += 1
  addCurrent()
  output

populateField = ($field, value) ->
  $field.val(value)
  flash($field)

populateLimits = (limits) ->
  for limit in limits
    $limit = addLimit()
    $limit.find("select.limit-name").val(limit[0])
    $limit.find("select.comparison").val(limit[1])
    $limit.find("input.value-box").val(limit[2])
    flash($limit.find("select, input"))

populateGroupLimits = (groupLimit) ->
  $group = $("#group-limits .limit")
  groupLimit = "none" unless groupLimit
  $group.find("select.limit-name").val(groupLimit)
  flash($group.find("select"))

flash = ($e) ->
  $e.addClass("flash")
  setTimeout (-> $e.removeClass("flash")), 0

addLimit = ->
  $newLimit = $("#limit-template").clone()
  $newLimit.removeAttr("id")
  $newLimit.insertBefore($("#add-limit").parent())
  $newLimit

removeLimit = (e) ->
  $(e.target).parent().remove()
  writeUrlFromFields()

writeUrlFromFields = ->
  metric = encodeURIComponent(dedentMetric($("#metric-box").val()))
  from = encodeURIComponent($("#from-box").val())
  untilVal = encodeURIComponent($("#until-box").val())
  limits = []
  for l in $("#limits .limit")
    $l = $(l)
    name = $l.find("select.limit-name").val()
    comp = $l.find("select.comparison").val()
    val = $l.find("input.value-box").val()
    limits.push(name+comp+val)
  limit = encodeURIComponent(limits.join(","))
  query = "/check?metric=#{metric}&from=#{from}&until=#{untilVal}&limit=#{limit}"
  $groupLimit = $("#group-limits .limit")
  groupLimitType = $groupLimit.find("select.limit-name").val()
  if groupLimitType != "none"
    query += "&group_limit=#{groupLimitType}"
  if $("#include-empty-targets-checkbox").is(":checked")
    query += "&include_empty_targets=true"
  $url = $("#url-box")
  $url.val(query)
  flash($url)

dedentMetric = (m) -> m.replace(/\s/g, "")

doQuery = ->
  $("#query-result").hide()
  url = $("#url-box").val()
  unless url.match(/^\/check?/)
    $("#query-result").removeClass("good")
      .addClass("bad")
      .text("Bad query string.")
      .show()
    resizeQueryResult()
    return

  spinner = new Spinner(spinjsSettings).spin($("#query-wrapper")[0])
  $.ajax
    cache: false
    error: (jqXHR) ->
      $("#query-result").removeClass("good")
        .addClass("bad")
        .text(jqXHR.responseText)
    success: (data, _, jqXHR) ->
      $("#query-result").removeClass("bad")
        .addClass("good")
        .text(data)
    type: "GET"
    url: url
    complete: ->
      spinner.stop()
      $("#query-result").show()
      resizeQueryResult()

window.resizeQueryResult = ->
  $("#query-result").height("0px")
  scrollHeight = $("#query-result")[0].scrollHeight
  $("#query-result").height(Math.max(100, scrollHeight))

$ ->
  $("#url-box").on "input", populateFieldsFromUrl
  $("#breakdown").on "input", "textarea,input", writeUrlFromFields
  $("#breakdown").on "change", "select", writeUrlFromFields
  $("#breakdown").on "click", "label", writeUrlFromFields
  $("#add-limit").on "click", addLimit
  $("#limits").on "click", ".remove-limit", removeLimit
  $("#query").on "click", doQuery
