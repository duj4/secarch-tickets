const UPDATE_MAX_LENGTH = 500
const ticketBrowseURL = document.body.dataset.ticketBrowseUrl.replace(/\/+$/, "")

let allData = []
let currentPage = 1
let pageSize = 10
let sortField = "expected_date"
let sortAsc = true
let statusFilter = "open"
let selectedDepartments = new Set()
let activeTicketNumber = ""
let updateSubmitInFlight = false
let refreshInFlight = false
let currentSyncStatus = null
let syncNetworkError = false
let retryUntil = 0
let retryTimer = null
let statisticsRequestID = 0

const updateIcon = `
  <svg class="pointer-events-none h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
    <path d="M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4Z" />
    <path d="M8 9h8M8 13h5" />
  </svg>`

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>'"]/g, character => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", "\"": "&quot;"
  })[character])
}

function formatDateLocal(date) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

function today() {
  const now = new Date()
  return new Date(now.getFullYear(), now.getMonth(), now.getDate())
}

function todayStr() { return formatDateLocal(today()) }
function dateOnly(value) { return value ? String(value).slice(0, 10) : "" }

function formatDateTime(value) {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "—"
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit"
  }).format(date)
}

function formatDuration(totalSeconds) {
  const seconds = Math.max(0, Math.ceil(totalSeconds))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainder = seconds % 60
  return remainder === 0 ? `${minutes}m` : `${minutes}m ${remainder}s`
}

function ticketSystems(ticket) {
  if (Array.isArray(ticket.cmdb_system_name)) return ticket.cmdb_system_name.join(", ") || "—"
  return ticket.cmdb_system_name || "—"
}

function ticketSystemsDisplay(ticket) {
  const systems = Array.isArray(ticket.cmdb_system_name) ? ticket.cmdb_system_name : [ticket.cmdb_system_name]
  return systems
    .map(system => String(system || "").replace(/\s*\(.*$/, "").trim())
    .filter(Boolean)
    .join(", ") || "—"
}

function ticketSummaryDisplay(ticket) {
  const summary = String(ticket.summary || "").trim()
  return summary.replace(/^SecDesign Case Review\s*-\s*Ad[\s-]*hoc\s*-\s*/i, "").trim() || summary || "—"
}

function ticketLink(ticketNumber) {
  return `${ticketBrowseURL}/${encodeURIComponent(ticketNumber)}`
}

function ticketDepartment(ticket) {
  return String(ticket.department || "").trim() || "Unassigned"
}

function getBaseFilteredTickets() {
  const search = document.getElementById("search").value.trim().toLowerCase()
  return allData.filter(ticket => {
    const searchable = [
      ticket.ticket_number, ticket.summary, ticketSystems(ticket), ticketDepartment(ticket),
      ticket.reporter, ticket.assignee
    ].join(" ").toLowerCase()
    const isClosed = Boolean(ticket.ticket_closed_at)
    const matchesStatus = statusFilter === "all" ||
      (statusFilter === "open" && !isClosed) ||
      (statusFilter === "closed" && isClosed)
    return searchable.includes(search) && matchesStatus
  })
}

function getFilteredTickets() {
  const tickets = getBaseFilteredTickets()
  if (selectedDepartments.size === 0) return tickets
  return tickets.filter(ticket => selectedDepartments.has(ticketDepartment(ticket)))
}

function departmentOptions(tickets) {
  const counts = new Map()
  tickets.forEach(ticket => {
    const department = ticketDepartment(ticket)
    counts.set(department, (counts.get(department) || 0) + 1)
  })
  return [...counts.entries()].sort(([left], [right]) => {
    if (left === "Unassigned") return 1
    if (right === "Unassigned") return -1
    return left.localeCompare(right, undefined, { sensitivity: "base", numeric: true })
  })
}

function renderDepartmentFilters(baseTickets) {
  const options = departmentOptions(baseTickets)
  const available = new Set(options.map(([department]) => department))
  selectedDepartments.forEach(department => {
    if (!available.has(department)) selectedDepartments.delete(department)
  })

  const allActive = selectedDepartments.size === 0
  const allClasses = allActive
    ? "border-blue-600 bg-blue-600 text-white shadow-sm"
    : "border-slate-300 bg-white text-slate-600 hover:border-blue-300 hover:bg-blue-50 hover:text-blue-700"
  const tags = [`
    <button type="button" data-department-all="true" aria-pressed="${allActive}"
      class="inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-xs font-semibold transition ${allClasses}">
      All <span class="opacity-75">${baseTickets.length}</span>
    </button>`]

  options.forEach(([department, count]) => {
    const active = selectedDepartments.has(department)
    const classes = active
      ? "border-violet-600 bg-violet-600 text-white shadow-sm"
      : "border-slate-300 bg-white text-slate-600 hover:border-violet-300 hover:bg-violet-50 hover:text-violet-700"
    tags.push(`
      <button type="button" data-department-filter="${escapeHTML(department)}" aria-pressed="${active}"
        class="inline-flex h-8 items-center gap-1.5 rounded-full border px-3 text-xs font-semibold transition ${classes}">
        ${escapeHTML(department)} <span class="opacity-75">${count}</span>
      </button>`)
  })

  document.getElementById("departmentTags").innerHTML = tags.join("")
  const selectedCount = selectedDepartments.size
  document.getElementById("departmentSelectionMeta").textContent = selectedCount === 0
    ? "All departments"
    : `${selectedCount} selected`
  return options
}

function showDepartmentColumn(options) {
  const effectiveCount = selectedDepartments.size === 0 ? options.length : selectedDepartments.size
  return effectiveCount > 1
}

function sortedTickets(tickets) {
  return [...tickets].sort((first, second) => {
    let left = first[sortField] ?? ""
    let right = second[sortField] ?? ""
    if (sortField === "cmdb_system_name") {
      left = ticketSystems(first)
      right = ticketSystems(second)
    }
    if (sortField === "expected_date") {
      left = new Date(left || 0).getTime()
      right = new Date(right || 0).getTime()
    }
    const comparison = typeof left === "number" && typeof right === "number"
      ? left - right
      : String(left).localeCompare(String(right), undefined, { sensitivity: "base", numeric: true })
    if (comparison !== 0) return sortAsc ? comparison : -comparison
    return String(first.ticket_number).localeCompare(String(second.ticket_number))
  })
}

function expectedDateDisplay(ticket) {
  const value = dateOnly(ticket.expected_date)
  if (!value || ticket.ticket_closed_at) {
    return `<span class="font-medium text-slate-700">${escapeHTML(value || "—")}</span>`
  }
  const due = new Date(`${value}T00:00:00`)
  const days = Math.ceil((due.getTime() - today().getTime()) / 86400000)
  if (days < 0) {
    return `<div><span class="font-semibold text-red-600">${escapeHTML(value)}</span><span class="mt-0.5 block text-xs text-red-500">Overdue</span></div>`
  }
  if (days <= 7) {
    return `<div><span class="font-semibold text-amber-600">${escapeHTML(value)}</span><span class="mt-0.5 block text-xs text-amber-500">Due soon</span></div>`
  }
  return `<span class="font-medium text-slate-700">${escapeHTML(value)}</span>`
}

function statusBadge(ticket) {
  if (ticket.ticket_closed_at) {
    return `<span class="inline-flex rounded-full bg-slate-100 px-2.5 py-1 text-xs font-semibold text-slate-500" title="Closed at ${escapeHTML(formatDateTime(ticket.ticket_closed_at))}">Closed</span>`
  }
  return `<span class="inline-flex rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700">Open</span>`
}

function updateActionButton(ticket) {
  const count = Number(ticket.update_count || 0)
  const badge = count > 0
    ? `<span class="pointer-events-none absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-violet-600 px-1 text-[10px] font-bold leading-none text-white">${count}</span>`
    : ""
  return `
    <button type="button" data-action="view" data-ticket="${escapeHTML(ticket.ticket_number)}"
      class="relative flex h-10 w-10 touch-manipulation items-center justify-center rounded-lg text-violet-600 transition hover:bg-violet-50 hover:text-violet-700 focus:outline-none focus:ring-2 focus:ring-violet-500"
      aria-label="View ${escapeHTML(ticket.ticket_number)}" title="View">
      ${updateIcon}${badge}
    </button>`
}

function render() {
  const tbody = document.getElementById("tbody")
  const departmentFilters = renderDepartmentFilters(getBaseFilteredTickets())
  const departmentVisible = showDepartmentColumn(departmentFilters)
  document.getElementById("departmentColumnHeader").classList.toggle("hidden", !departmentVisible)
  document.getElementById("ticketsTable").style.minWidth = departmentVisible ? "1500px" : "1310px"
  const filtered = sortedTickets(getFilteredTickets())
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize))
  currentPage = Math.min(currentPage, totalPages)
  const start = (currentPage - 1) * pageSize
  const pageData = filtered.slice(start, start + pageSize)
  const firstVisible = filtered.length === 0 ? 0 : start + 1
  const lastVisible = Math.min(start + pageSize, filtered.length)

  document.getElementById("pageInfo").textContent = filtered.length === 0
    ? "Showing 0 tickets"
    : `Showing ${firstVisible}–${lastVisible} of ${filtered.length} · Page ${currentPage} of ${totalPages}`
  document.getElementById("prevBtn").disabled = currentPage <= 1
  document.getElementById("nextBtn").disabled = currentPage >= totalPages
  document.querySelectorAll("[id^='sort-']").forEach(element => { element.textContent = "" })
  const activeSort = document.getElementById(`sort-${sortField}`)
  if (activeSort) activeSort.textContent = sortAsc ? "↑" : "↓"

  if (pageData.length === 0) {
    tbody.innerHTML = `<tr><td colspan="${departmentVisible ? 9 : 8}" class="px-4 py-16 text-center">
      <div class="text-sm font-medium text-slate-600">No tickets found</div>
      <div class="mt-1 text-xs text-slate-400">Try adjusting your search, status, or department filter.</div>
    </td></tr>`
    return
  }

  tbody.innerHTML = pageData.map(ticket => {
    const ticketNumber = escapeHTML(ticket.ticket_number)
    const ticketURL = ticketLink(ticket.ticket_number)
    const departmentCell = departmentVisible
      ? `<td class="px-4 py-3 align-middle text-slate-700">${escapeHTML(ticketDepartment(ticket))}</td>`
      : ""
    return `<tr data-id="${ticketNumber}" class="transition-colors hover:bg-slate-50">
      <td class="px-4 py-3 align-middle"><a href="${ticketURL}" target="_blank" rel="noopener noreferrer" class="font-semibold text-blue-600 hover:underline">${ticketNumber}</a></td>
      <td class="px-4 py-3 align-middle"><p class="break-words leading-5 text-slate-700" title="${escapeHTML(ticket.summary)}">${escapeHTML(ticketSummaryDisplay(ticket))}</p></td>
      <td class="px-4 py-3 align-middle"><p class="truncate whitespace-nowrap leading-5 text-slate-700" title="${escapeHTML(ticketSystems(ticket))}">${escapeHTML(ticketSystemsDisplay(ticket))}</p></td>
      ${departmentCell}
      <td class="px-3 py-3 text-center align-middle text-slate-700">${escapeHTML(ticket.reporter || "—")}</td>
      <td class="px-3 py-3 text-center align-middle text-slate-700">${escapeHTML(ticket.assignee || "—")}</td>
      <td class="px-3 py-3 align-middle">${expectedDateDisplay(ticket)}</td>
      <td class="px-3 py-3 text-center align-middle">${statusBadge(ticket)}</td>
      <td class="px-3 py-2 align-middle"><div class="flex items-center justify-center">${updateActionButton(ticket)}</div></td>
    </tr>`
  }).join("")
}

function renderSkeleton() {
  const departmentCell = selectedDepartments.size === 1
    ? ""
    : `<td class="px-4 py-4"><div class="h-4 w-32 animate-pulse rounded bg-slate-200"></div></td>`
  const cells = `
    <td class="px-4 py-4"><div class="h-4 w-20 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-4 py-4"><div class="h-4 w-full animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-4 py-4"><div class="h-4 w-36 animate-pulse rounded bg-slate-200"></div></td>
    ${departmentCell}
    <td class="px-3 py-4"><div class="mx-auto h-4 w-16 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="mx-auto h-4 w-16 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="h-4 w-20 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="mx-auto h-6 w-16 animate-pulse rounded-full bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="mx-auto h-8 w-8 animate-pulse rounded bg-slate-200"></div></td>`
  document.getElementById("tbody").innerHTML = Array.from({ length: Math.min(pageSize, 5) }, () => `<tr>${cells}</tr>`).join("")
}

function updateStats() {
  document.getElementById("totalCount").textContent = String(allData.length)
  document.getElementById("openCount").textContent = String(allData.filter(ticket => !ticket.ticket_closed_at).length)
}

async function parseResponseBody(response) { return response.json().catch(() => ({})) }
function responseMessage(body, fallback) { return body.message || body.error || fallback }
function retrySecondsRemaining() { return Math.max(0, Math.ceil((retryUntil - Date.now()) / 1000)) }

function renderSyncState() {
  const button = document.getElementById("refreshBtn")
  const label = document.getElementById("refreshLabel")
  const banner = document.getElementById("syncBanner")
  const bannerText = document.getElementById("syncBannerText")
  const remaining = retrySecondsRemaining()
  const lastSuccess = currentSyncStatus?.last_success_at
  const lastSuccessText = lastSuccess ? formatDateTime(lastSuccess) : "never"

  document.getElementById("lastRefreshed").textContent = lastSuccess
    ? `Last synchronized ${lastSuccessText}`
    : "No successful synchronization yet"
  button.disabled = refreshInFlight || remaining > 0
  document.getElementById("refreshIcon").classList.toggle("animate-spin", refreshInFlight)
  label.textContent = refreshInFlight ? "Refreshing..." : remaining > 0 ? `Retry in ${formatDuration(remaining)}` : "Refresh"
  banner.className = "mb-4 hidden rounded-xl border px-4 py-3 text-sm"
  bannerText.textContent = ""
  if (!currentSyncStatus) return

  if (syncNetworkError) {
    banner.className = "mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800"
    bannerText.textContent = `Unable to reach the synchronization service. Existing ticket data is still available. Last successful sync: ${lastSuccessText}. Please try again or contact the administrator.`
    return
  }

  const failures = Number(currentSyncStatus.consecutive_failures || 0)
  if (currentSyncStatus.status === "circuit_open" && remaining > 0) {
    banner.className = "mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800"
    bannerText.textContent = `CMDB synchronization failed ${failures} consecutive times. Existing ticket data is still available. Try again in ${formatDuration(remaining)}. Last successful sync: ${lastSuccessText}. Please contact the administrator if the issue continues.`
  } else if (currentSyncStatus.status === "circuit_open" || currentSyncStatus.status === "half_open") {
    banner.className = "mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800"
    bannerText.textContent = `CMDB synchronization can be retried now. Existing ticket data is still available. Last successful sync: ${lastSuccessText}. Click Refresh to start one recovery attempt.`
  } else if (failures > 0) {
    banner.className = "mb-4 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800"
    const retryText = remaining > 0 ? ` Try again in ${formatDuration(remaining)}.` : " You can try again now."
    bannerText.textContent = `CMDB synchronization failed ${failures} consecutive time${failures === 1 ? "" : "s"}. Existing ticket data is still available.${retryText} Last successful sync: ${lastSuccessText}.`
  }
}

function setSyncStatus(sync) {
  if (!sync || typeof sync !== "object") return
  syncNetworkError = false
  currentSyncStatus = { ...sync }
  retryUntil = Date.now() + Math.max(0, Number(sync.retry_after_seconds || 0)) * 1000
  renderSyncState()
  if (retryTimer) clearInterval(retryTimer)
  retryTimer = null
  if (retryUntil > Date.now()) {
    retryTimer = setInterval(() => {
      if (retrySecondsRemaining() <= 0) {
        clearInterval(retryTimer)
        retryTimer = null
        retryUntil = 0
        if (currentSyncStatus.status === "circuit_open") currentSyncStatus.status = "half_open"
        if (currentSyncStatus.status === "backoff" || currentSyncStatus.status === "cooldown") currentSyncStatus.status = "ready"
      }
      renderSyncState()
    }, 1000)
  }
}

function showSyncNetworkError() {
  currentSyncStatus = currentSyncStatus || { status: "ready", last_success_at: null, consecutive_failures: 0, retry_after_seconds: 0 }
  syncNetworkError = true
  renderSyncState()
}

async function loadTickets({ resetPage = true } = {}) {
  if (allData.length === 0) renderSkeleton()
  try {
    const response = await fetch("/api/tickets")
    const body = await parseResponseBody(response)
    if (!response.ok) throw new Error(responseMessage(body, "Failed to load tickets"))
    allData = Array.isArray(body.tickets) ? body.tickets : []
    if (resetPage) currentPage = 1
    updateStats()
    render()
    setSyncStatus(body.sync)
  } catch (error) {
    console.error(error)
    showToast(error.message || "Failed to load tickets", "error")
    if (allData.length === 0) {
      document.getElementById("tbody").innerHTML = `<tr><td colspan="9" class="px-4 py-12 text-center text-sm text-red-600">Failed to load tickets</td></tr>`
    }
  }
}

async function refreshTickets() {
  if (refreshInFlight || retrySecondsRemaining() > 0) return
  refreshInFlight = true
  renderSyncState()
  try {
    const response = await fetch("/api/tickets/refresh", { method: "POST" })
    const body = await parseResponseBody(response)
    if (body.sync) setSyncStatus(body.sync)
    if (!response.ok) {
      const message = responseMessage(body, "CMDB synchronization failed")
      showToast(message, response.status === 429 && body.sync?.status === "cooldown" ? "info" : "error")
      return
    }
    await loadTickets({ resetPage: false })
    showToast(`Synchronization complete${body.ticket_count ? ` · ${body.ticket_count} tickets updated` : ""}`, "success")
  } catch (error) {
    console.error(error)
    showSyncNetworkError()
    showToast("Unable to start CMDB synchronization", "error")
  } finally {
    refreshInFlight = false
    renderSyncState()
  }
}

function openModal(id) {
  const modal = document.getElementById(id)
  modal.classList.remove("hidden")
  modal.classList.add("flex")
  document.body.classList.add("overflow-hidden")
}

function closeModal(id) {
  const modal = document.getElementById(id)
  modal.classList.add("hidden")
  modal.classList.remove("flex")
  if (!document.querySelector("[data-modal]:not(.hidden)")) document.body.classList.remove("overflow-hidden")
}

function showInlineError(id, message) {
  const element = document.getElementById(id)
  element.textContent = message || ""
  element.classList.toggle("hidden", !message)
}

function setSubmitLoading(buttonID, loading, loadingText, normalText) {
  const button = document.getElementById(buttonID)
  button.disabled = loading
  button.textContent = loading ? loadingText : normalText
}

function populateTicketDetails(ticket) {
  const link = document.getElementById("detailTicketLink")
  link.textContent = ticket.ticket_number
  link.href = ticketLink(ticket.ticket_number)
  const isClosed = Boolean(ticket.ticket_closed_at)
  const status = document.getElementById("detailStatus")
  status.textContent = isClosed ? "Closed" : "Open"
  status.className = isClosed
    ? "rounded-full bg-slate-100 px-2.5 py-1 text-xs font-semibold text-slate-500"
    : "rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700"
  const expectedInput = document.getElementById("detailExpectedDate")
  expectedInput.min = todayStr()
  expectedInput.value = dateOnly(ticket.expected_date)
  expectedInput.disabled = isClosed
  const expectedSubmit = document.getElementById("expectedDateSubmitBtn")
  expectedSubmit.disabled = isClosed
  expectedSubmit.classList.toggle("hidden", isClosed)
  showInlineError("expectedDateError", "")
  document.getElementById("detailReporter").textContent = ticket.reporter || "—"
  document.getElementById("detailAssignee").textContent = ticket.assignee || "—"
  document.getElementById("detailSystem").textContent = ticketSystems(ticket)
  document.getElementById("detailDepartment").textContent = ticket.department || "Unassigned"
  document.getElementById("detailSummary").textContent = ticket.summary || "—"
  document.getElementById("detailClosedRow").classList.toggle("hidden", !isClosed)
  document.getElementById("detailClosedAt").textContent = formatDateTime(ticket.ticket_closed_at)
}

async function openTicketDetails(ticketNumber) {
  const ticket = allData.find(item => item.ticket_number === ticketNumber)
  if (!ticket) return showToast("Ticket not found", "error")
  activeTicketNumber = ticketNumber
  populateTicketDetails(ticket)
  document.getElementById("updateContent").value = ""
  updateCharacterCounter()
  showInlineError("updateError", "")
  openModal("detailModal")
  await loadTicketUpdates(ticketNumber)
}

function renderUpdates(updates) {
  const list = document.getElementById("updatesList")
  list.replaceChildren()
  document.getElementById("updatesMeta").textContent = `${updates.length} update${updates.length === 1 ? "" : "s"}`
  if (updates.length === 0) {
    const empty = document.createElement("div")
    empty.className = "rounded-xl border border-dashed border-slate-300 px-4 py-7 text-center text-sm text-slate-400"
    empty.textContent = "No local updates yet."
    list.appendChild(empty)
    return
  }
  updates.forEach(update => {
    const item = document.createElement("article")
    item.className = "relative rounded-xl border border-slate-200 bg-white p-4 pl-5"
    const marker = document.createElement("span")
    marker.className = "absolute left-0 top-4 h-8 w-1 rounded-r bg-blue-500"
    const timestamp = document.createElement("time")
    timestamp.className = "text-xs font-medium text-slate-400"
    timestamp.dateTime = update.created_at
    timestamp.textContent = formatDateTime(update.created_at)
    const content = document.createElement("p")
    content.className = "mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-slate-700"
    content.textContent = update.content
    item.append(marker, timestamp, content)
    list.appendChild(item)
  })
}

async function loadTicketUpdates(ticketNumber) {
  const list = document.getElementById("updatesList")
  list.innerHTML = `<div class="h-20 animate-pulse rounded-xl bg-slate-100"></div>`
  document.getElementById("updatesMeta").textContent = "Loading..."
  try {
    const response = await fetch(`/api/tickets/${encodeURIComponent(ticketNumber)}/updates`)
    const body = await parseResponseBody(response)
    if (!response.ok) throw new Error(responseMessage(body, "Failed to load updates"))
    if (activeTicketNumber === ticketNumber) renderUpdates(Array.isArray(body.updates) ? body.updates : [])
  } catch (error) {
    console.error(error)
    document.getElementById("updatesMeta").textContent = ""
    list.innerHTML = `<p class="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700">${escapeHTML(error.message || "Failed to load updates")}</p>`
  }
}

function updateCharacterCounter() {
  const count = Array.from(document.getElementById("updateContent").value).length
  const counter = document.getElementById("updateCounter")
  counter.textContent = `${count} / ${UPDATE_MAX_LENGTH}`
  counter.classList.toggle("text-red-600", count > UPDATE_MAX_LENGTH)
  counter.classList.toggle("text-slate-400", count <= UPDATE_MAX_LENGTH)
}

async function addTicketUpdate(event) {
  event.preventDefault()
  if (updateSubmitInFlight) return
  const content = document.getElementById("updateContent").value.trim()
  const length = Array.from(content).length
  showInlineError("updateError", "")
  if (!content) return showInlineError("updateError", "Update content is required.")
  if (length > UPDATE_MAX_LENGTH) return showInlineError("updateError", `Update must be ${UPDATE_MAX_LENGTH} characters or fewer.`)
  updateSubmitInFlight = true
  setSubmitLoading("updateSubmitBtn", true, "Adding...", "Add Update")
  try {
    const response = await fetch(`/api/tickets/${encodeURIComponent(activeTicketNumber)}/updates`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ content })
    })
    const body = await parseResponseBody(response)
    if (!response.ok) return showInlineError("updateError", responseMessage(body, "Failed to add update"))
    const ticket = allData.find(item => item.ticket_number === activeTicketNumber)
    if (ticket && body.update) {
      ticket.update_count = Number(ticket.update_count || 0) + 1
      ticket.latest_update_at = body.update.created_at
    }
    document.getElementById("updateContent").value = ""
    updateCharacterCounter()
    render()
    await loadTicketUpdates(activeTicketNumber)
    showToast("Update added", "success")
  } catch (error) {
    console.error(error)
    showInlineError("updateError", "Network error. Please try again.")
  } finally {
    updateSubmitInFlight = false
    setSubmitLoading("updateSubmitBtn", false, "Adding...", "Add Update")
  }
}

async function updateExpectedDate(event) {
  event.preventDefault()
  const ticket = allData.find(item => item.ticket_number === activeTicketNumber)
  if (!ticket || ticket.ticket_closed_at) return
  const expectedDate = document.getElementById("detailExpectedDate").value
  showInlineError("expectedDateError", "")
  if (!expectedDate) return showInlineError("expectedDateError", "Expected date is required.")
  setSubmitLoading("expectedDateSubmitBtn", true, "Saving...", "Save")
  try {
    const response = await fetch(`/api/tickets/${encodeURIComponent(activeTicketNumber)}/expected-date`, {
      method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ expected_date: expectedDate })
    })
    const body = await parseResponseBody(response)
    if (!response.ok) return showInlineError("expectedDateError", responseMessage(body, "Failed to update date"))
    ticket.expected_date = expectedDate
    render()
    showToast("Expected date updated", "success")
  } catch (error) {
    console.error(error)
    showInlineError("expectedDateError", "Network error. Please try again.")
  } finally {
    setSubmitLoading("expectedDateSubmitBtn", false, "Saving...", "Save")
  }
}

function showToast(message, type = "success") {
  const colors = {
    success: "border-emerald-200 bg-emerald-50 text-emerald-800",
    error: "border-red-200 bg-red-50 text-red-800",
    info: "border-slate-200 bg-white text-slate-700"
  }
  const toast = document.createElement("div")
  toast.className = `max-w-sm translate-y-2 rounded-xl border px-4 py-3 text-sm font-medium opacity-0 shadow-lg transition ${colors[type] || colors.info}`
  toast.textContent = message
  document.getElementById("toastContainer").appendChild(toast)
  requestAnimationFrame(() => toast.classList.remove("translate-y-2", "opacity-0"))
  setTimeout(() => {
    toast.classList.add("translate-y-2", "opacity-0")
    setTimeout(() => toast.remove(), 250)
  }, 2600)
}

function updateStatusButtons() {
  document.querySelectorAll("[data-status-filter]").forEach(button => {
    const active = button.dataset.statusFilter === statusFilter
    button.classList.toggle("bg-white", active)
    button.classList.toggle("text-slate-900", active)
    button.classList.toggle("shadow-sm", active)
    button.classList.toggle("text-slate-500", !active)
  })
}

function setSort(field) {
  if (sortField === field) sortAsc = !sortAsc
  else { sortField = field; sortAsc = true }
  render()
}

function closedTicketsInRange(startDate, endDate) {
  const start = new Date(`${startDate}T00:00:00`)
  const endExclusive = new Date(`${endDate}T00:00:00`)
  endExclusive.setDate(endExclusive.getDate() + 1)
  return allData.filter(ticket => {
    const closedAt = ticket.ticket_closed_at ? new Date(ticket.ticket_closed_at) : null
    return closedAt && closedAt >= start && closedAt < endExclusive
  })
}

function uniqueWorksheetName(department, usedNames) {
  let base = String(department || "Unassigned")
    .replace(/[\\*?:\[\]]/g, "-")
    .split("/").join("-")
    .replace(/^'+|'+$/g, "")
    .trim() || "Unassigned"
  base = base.slice(0, 31)
  let candidate = base
  let suffixNumber = 2
  while (usedNames.has(candidate.toLowerCase())) {
    const suffix = ` (${suffixNumber})`
    candidate = `${base.slice(0, 31 - suffix.length)}${suffix}`
    suffixNumber += 1
  }
  usedNames.add(candidate.toLowerCase())
  return candidate
}

function configureWorksheet(sheet) {
  sheet.columns = [
    { header: "Ticket", key: "ticket", width: 15 },
    { header: "Summary", key: "summary", width: 50 },
    { header: "System", key: "system", width: 30 },
    { header: "Reporter", key: "reporter", width: 15 },
    { header: "Assignee", key: "assignee", width: 15 },
    { header: "Expected", key: "expected", width: 15 },
    { header: "Closed At", key: "closed", width: 25 },
    { header: "Updates", key: "updates", width: 12 },
    { header: "Latest Update", key: "latestUpdate", width: 25 },
    { header: "Status", key: "status", width: 12 }
  ]
  sheet.getRow(1).eachCell(cell => {
    cell.font = { bold: true }
    cell.alignment = { horizontal: "center" }
    cell.fill = { type: "pattern", pattern: "solid", fgColor: { argb: "FFE5E7EB" } }
  })
  sheet.views = [{ state: "frozen", ySplit: 1 }]
}

function addTicketRows(sheet, tickets) {
  tickets.forEach((ticket, index) => {
    const row = sheet.addRow({
      ticket: ticket.ticket_number, summary: ticket.summary, system: ticketSystems(ticket),
      reporter: ticket.reporter, assignee: ticket.assignee || "—", expected: dateOnly(ticket.expected_date),
      closed: formatDateTime(ticket.ticket_closed_at), updates: Number(ticket.update_count || 0),
      latestUpdate: formatDateTime(ticket.latest_update_at), status: "CLOSED"
    })
    row.eachCell(cell => { cell.alignment = { horizontal: "center", vertical: "middle" } })
    if (index % 2 === 0) row.eachCell(cell => {
      cell.fill = { type: "pattern", pattern: "solid", fgColor: { argb: "FFF8FAFC" } }
    })
  })
}

async function exportClosedTickets(startDate, endDate) {
  if (typeof ExcelJS === "undefined") return showToast("Export library is unavailable", "error")
  const filtered = closedTicketsInRange(startDate, endDate)
  if (filtered.length === 0) return showToast("No closed tickets in this date range", "error")
  try {
    const workbook = new ExcelJS.Workbook()
    const grouped = new Map()
    filtered.forEach(ticket => {
      const department = String(ticket.department || "").trim() || "Unassigned"
      if (!grouped.has(department)) grouped.set(department, [])
      grouped.get(department).push(ticket)
    })
    const usedNames = new Set()
    const departments = [...grouped.keys()].sort((left, right) => {
      if (left === "Unassigned") return 1
      if (right === "Unassigned") return -1
      return left.localeCompare(right)
    })
    departments.forEach(department => {
      const sheet = workbook.addWorksheet(uniqueWorksheetName(department, usedNames))
      configureWorksheet(sheet)
      addTicketRows(sheet, grouped.get(department))
    })
    const buffer = await workbook.xlsx.writeBuffer()
    const url = URL.createObjectURL(new Blob([buffer]))
    const link = document.createElement("a")
    link.href = url
    link.download = `closed_secarch_tickets_${startDate}_to_${endDate}.xlsx`
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
    showToast("Export complete", "success")
  } catch (error) {
    console.error(error)
    showToast("Export failed", "error")
  }
}

async function updateClosedCount() {
  const start = document.getElementById("exportStart").value
  const end = document.getElementById("exportEnd").value
  const element = document.getElementById("exportCount")
  if (!start || !end || start > end) {
    element.textContent = "Select a valid date range."
    return
  }
  const requestID = ++statisticsRequestID
  element.textContent = "Checking closed ticket count..."
  try {
    const response = await fetch(`/api/statistics/closed?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`)
    const body = await parseResponseBody(response)
    if (!response.ok) throw new Error(responseMessage(body, "Failed to load count"))
    if (requestID !== statisticsRequestID) return
    const count = Number(body.closed_count || 0)
    element.textContent = `${count} closed ticket${count === 1 ? "" : "s"} in this inclusive date range.`
  } catch (error) {
    console.error(error)
    if (requestID === statisticsRequestID) element.textContent = "Closed ticket count is temporarily unavailable."
  }
}

async function quickExport(days) {
  const end = today()
  const start = today()
  start.setDate(start.getDate() - days)
  document.getElementById("exportStart").value = formatDateLocal(start)
  document.getElementById("exportEnd").value = formatDateLocal(end)
  document.getElementById("exportPanel").classList.add("hidden")
  await exportClosedTickets(formatDateLocal(start), formatDateLocal(end))
}

async function confirmExport() {
  const start = document.getElementById("exportStart").value
  const end = document.getElementById("exportEnd").value
  if (!start || !end) return showToast("Select both export dates", "error")
  if (start > end) return showToast("Start date must be before end date", "error")
  document.getElementById("exportPanel").classList.add("hidden")
  await exportClosedTickets(start, end)
}

document.getElementById("search").addEventListener("input", () => { currentPage = 1; render() })
document.querySelectorAll("[data-status-filter]").forEach(button => {
  button.addEventListener("click", () => {
    statusFilter = button.dataset.statusFilter
    currentPage = 1
    updateStatusButtons()
    render()
  })
})
document.getElementById("departmentTags").addEventListener("click", event => {
  const allButton = event.target.closest("button[data-department-all]")
  const departmentButton = event.target.closest("button[data-department-filter]")
  if (!allButton && !departmentButton) return

  if (allButton) {
    selectedDepartments.clear()
  } else {
    const department = departmentButton.dataset.departmentFilter
    if (selectedDepartments.size === 0) selectedDepartments.add(department)
    else if (selectedDepartments.has(department)) selectedDepartments.delete(department)
    else selectedDepartments.add(department)
  }
  currentPage = 1
  render()
})
document.querySelectorAll("th[data-sort]").forEach(header => {
  header.querySelector("button").addEventListener("click", () => setSort(header.dataset.sort))
})
document.getElementById("pageSize").addEventListener("change", event => {
  pageSize = Number.parseInt(event.target.value, 10)
  currentPage = 1
  render()
})
document.getElementById("prevBtn").addEventListener("click", () => {
  if (currentPage > 1) { currentPage -= 1; render() }
})
document.getElementById("nextBtn").addEventListener("click", () => {
  const totalPages = Math.max(1, Math.ceil(getFilteredTickets().length / pageSize))
  if (currentPage < totalPages) { currentPage += 1; render() }
})
document.getElementById("tbody").addEventListener("click", event => {
  const target = event.target instanceof Element ? event.target : null
  const button = target?.closest("button[data-action='view']")
  if (!button) return
  event.preventDefault()
  openTicketDetails(button.dataset.ticket)
})
document.getElementById("refreshBtn").addEventListener("click", refreshTickets)
document.getElementById("expectedDateForm").addEventListener("submit", updateExpectedDate)
document.getElementById("updateSubmitBtn").addEventListener("click", addTicketUpdate)
document.getElementById("updateContent").addEventListener("input", updateCharacterCounter)
document.querySelectorAll("[data-close-modal]").forEach(button => {
  button.addEventListener("click", () => closeModal(button.dataset.closeModal))
})
document.querySelectorAll("[data-modal]").forEach(modal => {
  modal.addEventListener("click", event => { if (event.target === modal) closeModal(modal.id) })
})
document.getElementById("exportToggle").addEventListener("click", event => {
  event.stopPropagation()
  document.getElementById("exportPanel").classList.toggle("hidden")
  updateClosedCount()
})
document.querySelectorAll("[data-export-days]").forEach(button => {
  button.addEventListener("click", () => quickExport(Number(button.dataset.exportDays)))
})
document.getElementById("exportStart").addEventListener("change", updateClosedCount)
document.getElementById("exportEnd").addEventListener("change", updateClosedCount)
document.getElementById("exportConfirm").addEventListener("click", confirmExport)
document.addEventListener("click", event => {
  const panel = document.getElementById("exportPanel")
  if (!panel.contains(event.target) && !document.getElementById("exportToggle").contains(event.target)) panel.classList.add("hidden")
})
document.addEventListener("keydown", event => {
  if (event.key !== "Escape") return
  document.getElementById("exportPanel").classList.add("hidden")
  if (!document.getElementById("detailModal").classList.contains("hidden")) closeModal("detailModal")
})

document.getElementById("exportEnd").value = todayStr()
const defaultExportStart = today()
defaultExportStart.setDate(defaultExportStart.getDate() - 30)
document.getElementById("exportStart").value = formatDateLocal(defaultExportStart)
updateStatusButtons()
updateCharacterCounter()
loadTickets()
