const UPDATE_MAX_LENGTH = 500

let allData = []
let currentPage = 1
let pageSize = 10
let sortField = "expected_date"
let sortAsc = true
let statusFilter = "all"
let activeTicketNumber = ""
let editingTicketNumber = ""

const icons = {
  view: `
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path d="M2.5 12s3.5-6 9.5-6 9.5 6 9.5 6-3.5 6-9.5 6-9.5-6-9.5-6Z" />
      <circle cx="12" cy="12" r="2.5" />
    </svg>`,
  edit: `
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path d="M12 20h9" /><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4Z" />
    </svg>`,
  delete: `
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path d="M3 6h18M8 6V4h8v2m-9 0 1 14h8l1-14M10 10v6m4-6v6" />
    </svg>`,
  update: `
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path d="M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4Z" />
      <path d="M8 9h8M8 13h5" />
    </svg>`
}

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>'"]/g, character => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "'": "&#39;",
    "\"": "&quot;"
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

function todayStr() {
  return formatDateLocal(today())
}

function defaultExpectedDate(days = 30) {
  const date = today()
  date.setDate(date.getDate() + days)
  return formatDateLocal(date)
}

function dateOnly(value) {
  return value ? String(value).slice(0, 10) : ""
}

function formatDateTime(value) {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "—"
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit"
  }).format(date)
}

function formatShortDateTime(value) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit"
  }).format(date)
}

function ticketSystems(ticket) {
  if (Array.isArray(ticket.cmdb_system_name)) return ticket.cmdb_system_name.join(", ")
  return ticket.cmdb_system_name || "—"
}

function getFilteredTickets() {
  const search = document.getElementById("search").value.trim().toLowerCase()

  return allData.filter(ticket => {
    const searchable = [
      ticket.ticket_number,
      ticket.summary,
      ticketSystems(ticket),
      ticket.reporter,
      ticket.assignee
    ].join(" ").toLowerCase()

    const isClosed = Boolean(ticket.ticket_closed_at)
    const matchesStatus = statusFilter === "all" ||
      (statusFilter === "open" && !isClosed) ||
      (statusFilter === "closed" && isClosed)

    return searchable.includes(search) && matchesStatus
  })
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
      left = new Date(left || 0)
      right = new Date(right || 0)
    }
    if (typeof left === "string") left = left.toLowerCase()
    if (typeof right === "string") right = right.toLowerCase()

    if (left > right) return sortAsc ? 1 : -1
    if (left < right) return sortAsc ? -1 : 1
    return 0
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

function actionButton(action, ticketNumber, label, icon, extraClass = "") {
  return `
    <button type="button" data-action="${action}" data-ticket="${escapeHTML(ticketNumber)}"
      class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 transition hover:bg-slate-100 hover:text-slate-900 focus:outline-none focus:ring-2 focus:ring-blue-500 ${extraClass}"
      aria-label="${escapeHTML(label)}" title="${escapeHTML(label)}">
      ${icon}
    </button>`
}

function updatesCell(ticket) {
  const count = Number(ticket.update_count || 0)
  const ticketNumber = escapeHTML(ticket.ticket_number)
  if (count === 0) {
    return `
      <button type="button" data-action="updates" data-ticket="${ticketNumber}"
        class="inline-flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs font-medium text-slate-400 transition hover:bg-blue-50 hover:text-blue-600"
        title="Add update">
        ${icons.update}<span>Add</span>
      </button>`
  }

  return `
    <button type="button" data-action="updates" data-ticket="${ticketNumber}"
      class="group inline-flex flex-col items-start rounded-lg px-2 py-1.5 text-left transition hover:bg-blue-50"
      title="View ${count} update${count === 1 ? "" : "s"}">
      <span class="inline-flex items-center gap-1.5 text-xs font-semibold text-blue-600">${icons.update}${count}</span>
      <span class="mt-0.5 text-[11px] text-slate-400 group-hover:text-blue-500">${escapeHTML(formatShortDateTime(ticket.latest_update_at))}</span>
    </button>`
}

function render() {
  const tbody = document.getElementById("tbody")
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
    tbody.innerHTML = `
      <tr>
        <td colspan="9" class="px-4 py-16 text-center">
          <div class="text-sm font-medium text-slate-600">No tickets found</div>
          <div class="mt-1 text-xs text-slate-400">Try adjusting your search or status filter.</div>
        </td>
      </tr>`
    return
  }

  tbody.innerHTML = pageData.map(ticket => {
    const ticketNumber = escapeHTML(ticket.ticket_number)
    const ticketURL = `https://itsm.ai.ms.com.cn/projects/ITSM/queues/issue/${encodeURIComponent(ticket.ticket_number)}`
    const actions = [
      actionButton("view", ticket.ticket_number, "View details", icons.view),
      ticket.ticket_closed_at
        ? `<span class="flex h-8 w-8 items-center justify-center rounded-lg text-slate-300" title="Closed ticket cannot be edited">${icons.edit}</span>`
        : actionButton("edit", ticket.ticket_number, "Edit expected date", icons.edit),
      actionButton("delete", ticket.ticket_number, "Delete ticket", icons.delete, "hover:bg-red-50 hover:text-red-600")
    ].join("")

    return `
      <tr data-id="${ticketNumber}" class="transition-colors hover:bg-slate-50">
        <td class="px-4 py-3 align-top">
          <a href="${ticketURL}" target="_blank" rel="noopener noreferrer" class="font-semibold text-blue-600 hover:underline">${ticketNumber}</a>
        </td>
        <td class="px-4 py-3 align-top">
          <p class="line-clamp-2 leading-5 text-slate-700" title="${escapeHTML(ticket.summary)}">${escapeHTML(ticket.summary)}</p>
        </td>
        <td class="px-4 py-3 align-top">
          <p class="line-clamp-2 leading-5 text-slate-600" title="${escapeHTML(ticketSystems(ticket))}">${escapeHTML(ticketSystems(ticket))}</p>
        </td>
        <td class="px-3 py-3 text-center align-top text-slate-600">${escapeHTML(ticket.reporter || "—")}</td>
        <td class="px-3 py-3 text-center align-top text-slate-600">${escapeHTML(ticket.assignee || "—")}</td>
        <td class="px-3 py-3 align-top">${expectedDateDisplay(ticket)}</td>
        <td class="px-3 py-3 text-center align-top">${statusBadge(ticket)}</td>
        <td class="px-3 py-2 align-top">${updatesCell(ticket)}</td>
        <td class="px-3 py-2 align-top"><div class="flex items-center justify-center gap-1">${actions}</div></td>
      </tr>`
  }).join("")
}

function renderSkeleton() {
  const cells = `
    <td class="px-4 py-4"><div class="h-4 w-20 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-4 py-4"><div class="space-y-2"><div class="h-4 w-full animate-pulse rounded bg-slate-200"></div><div class="h-4 w-2/3 animate-pulse rounded bg-slate-200"></div></div></td>
    <td class="px-4 py-4"><div class="h-4 w-36 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="mx-auto h-4 w-16 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="mx-auto h-4 w-16 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="h-4 w-20 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="mx-auto h-6 w-16 animate-pulse rounded-full bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="h-8 w-20 animate-pulse rounded bg-slate-200"></div></td>
    <td class="px-3 py-4"><div class="mx-auto h-8 w-24 animate-pulse rounded bg-slate-200"></div></td>`

  document.getElementById("tbody").innerHTML = Array.from({ length: Math.min(pageSize, 5) }, () =>
    `<tr>${cells}</tr>`
  ).join("")
}

function updateStats() {
  document.getElementById("totalCount").textContent = String(allData.length)
  document.getElementById("openCount").textContent = String(allData.filter(ticket => !ticket.ticket_closed_at).length)
}

function setRefreshLoading(loading) {
  const button = document.getElementById("refreshBtn")
  button.disabled = loading
  document.getElementById("refreshLabel").textContent = loading ? "Refreshing..." : "Refresh"
  document.getElementById("refreshIcon").classList.toggle("animate-spin", loading)
}

async function responseError(response, fallback) {
  const body = await response.json().catch(() => ({}))
  return body.error || fallback
}

async function loadTickets() {
  setRefreshLoading(true)
  if (allData.length === 0) renderSkeleton()

  try {
    const response = await fetch("/api/tickets")
    if (!response.ok) throw new Error(await responseError(response, "Failed to load tickets"))

    const body = await response.json()
    allData = Array.isArray(body.tickets) ? body.tickets : []
    currentPage = 1
    updateStats()
    render()
    document.getElementById("lastRefreshed").textContent = `Last refreshed ${new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
  } catch (error) {
    console.error(error)
    showToast(error.message || "Failed to load tickets", "error")
    if (allData.length === 0) {
      document.getElementById("tbody").innerHTML = `<tr><td colspan="9" class="px-4 py-12 text-center text-sm text-red-600">Failed to load tickets</td></tr>`
    }
  } finally {
    setRefreshLoading(false)
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
  if (!document.querySelector("[data-modal]:not(.hidden)")) {
    document.body.classList.remove("overflow-hidden")
  }
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

function openAddModal() {
  showInlineError("addError", "")
  document.getElementById("newTicket").value = ""
  document.getElementById("newExpectedDate").min = todayStr()
  document.getElementById("newExpectedDate").value = defaultExpectedDate()
  openModal("addModal")
  setTimeout(() => document.getElementById("newTicket").focus(), 0)
}

async function addTicket(event) {
  event.preventDefault()
  const ticketNumber = document.getElementById("newTicket").value.trim().toUpperCase()
  const expectedDate = document.getElementById("newExpectedDate").value

  showInlineError("addError", "")
  if (!/^ITSM-\d+$/.test(ticketNumber)) {
    showInlineError("addError", "Enter a valid ticket number, for example ITSM-12345.")
    return
  }
  if (!expectedDate) {
    showInlineError("addError", "Expected date is required.")
    return
  }

  setSubmitLoading("addSubmitBtn", true, "Adding...", "Add Ticket")
  try {
    const response = await fetch("/api/tickets", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ticket_number: ticketNumber, expected_date: expectedDate })
    })
    if (!response.ok) {
      showInlineError("addError", await responseError(response, "Failed to add ticket"))
      return
    }

    closeModal("addModal")
    showToast("Ticket added", "success")
    await loadTickets()
  } catch (error) {
    console.error(error)
    showInlineError("addError", "Network error. Please try again.")
  } finally {
    setSubmitLoading("addSubmitBtn", false, "Adding...", "Add Ticket")
  }
}

function populateTicketDetails(ticket) {
  const ticketURL = `https://itsm.ai.ms.com.cn/projects/ITSM/queues/issue/${encodeURIComponent(ticket.ticket_number)}`
  const link = document.getElementById("detailTicketLink")
  link.textContent = ticket.ticket_number
  link.href = ticketURL

  const status = document.getElementById("detailStatus")
  status.textContent = ticket.ticket_closed_at ? "Closed" : "Open"
  status.className = ticket.ticket_closed_at
    ? "rounded-full bg-slate-100 px-2.5 py-1 text-xs font-semibold text-slate-500"
    : "rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700"

  document.getElementById("detailExpected").textContent = dateOnly(ticket.expected_date) || "—"
  document.getElementById("detailReporter").textContent = ticket.reporter || "—"
  document.getElementById("detailAssignee").textContent = ticket.assignee || "—"
  document.getElementById("detailSystem").textContent = ticketSystems(ticket)
  document.getElementById("detailSummary").textContent = ticket.summary || "—"

  const closedRow = document.getElementById("detailClosedRow")
  closedRow.classList.toggle("hidden", !ticket.ticket_closed_at)
  document.getElementById("detailClosedAt").textContent = formatDateTime(ticket.ticket_closed_at)
}

async function openTicketDetails(ticketNumber, focusUpdate = false) {
  const ticket = allData.find(item => item.ticket_number === ticketNumber)
  if (!ticket) {
    showToast("Ticket not found", "error")
    return
  }

  activeTicketNumber = ticketNumber
  populateTicketDetails(ticket)
  document.getElementById("updateContent").value = ""
  updateCharacterCounter()
  showInlineError("updateError", "")
  openModal("detailModal")
  await loadTicketUpdates(ticketNumber)

  if (focusUpdate) {
    document.getElementById("updatesSection").scrollIntoView({ behavior: "smooth", block: "start" })
    setTimeout(() => document.getElementById("updateContent").focus(), 150)
  }
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
    if (!response.ok) throw new Error(await responseError(response, "Failed to load updates"))
    const body = await response.json()
    if (activeTicketNumber !== ticketNumber) return
    renderUpdates(Array.isArray(body.updates) ? body.updates : [])
  } catch (error) {
    console.error(error)
    document.getElementById("updatesMeta").textContent = ""
    list.replaceChildren()
    const message = document.createElement("p")
    message.className = "rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700"
    message.textContent = error.message || "Failed to load updates"
    list.appendChild(message)
  }
}

function updateCharacterCounter() {
  const content = document.getElementById("updateContent").value
  const count = Array.from(content).length
  const counter = document.getElementById("updateCounter")
  counter.textContent = `${count} / ${UPDATE_MAX_LENGTH}`
  counter.classList.toggle("text-red-600", count > UPDATE_MAX_LENGTH)
  counter.classList.toggle("text-slate-400", count <= UPDATE_MAX_LENGTH)
}

async function addTicketUpdate(event) {
  event.preventDefault()
  const content = document.getElementById("updateContent").value.trim()
  const length = Array.from(content).length

  showInlineError("updateError", "")
  if (!content) {
    showInlineError("updateError", "Update content is required.")
    return
  }
  if (length > UPDATE_MAX_LENGTH) {
    showInlineError("updateError", `Update must be ${UPDATE_MAX_LENGTH} characters or fewer.`)
    return
  }

  setSubmitLoading("updateSubmitBtn", true, "Adding...", "Add Update")
  try {
    const response = await fetch(`/api/tickets/${encodeURIComponent(activeTicketNumber)}/updates`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content })
    })
    if (!response.ok) {
      showInlineError("updateError", await responseError(response, "Failed to add update"))
      return
    }

    const body = await response.json()
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
    setSubmitLoading("updateSubmitBtn", false, "Adding...", "Add Update")
  }
}

function openEditDate(ticketNumber) {
  const ticket = allData.find(item => item.ticket_number === ticketNumber)
  if (!ticket || ticket.ticket_closed_at) return

  editingTicketNumber = ticketNumber
  document.getElementById("editTicketNumber").textContent = ticketNumber
  document.getElementById("editExpectedDate").min = todayStr()
  document.getElementById("editExpectedDate").value = dateOnly(ticket.expected_date)
  showInlineError("editDateError", "")
  openModal("editModal")
  setTimeout(() => document.getElementById("editExpectedDate").focus(), 0)
}

async function updateExpectedDate(event) {
  event.preventDefault()
  const expectedDate = document.getElementById("editExpectedDate").value
  if (!expectedDate) {
    showInlineError("editDateError", "Expected date is required.")
    return
  }

  setSubmitLoading("editDateSubmitBtn", true, "Saving...", "Save")
  try {
    const response = await fetch(`/api/tickets/${encodeURIComponent(editingTicketNumber)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ expected_date: expectedDate })
    })
    if (!response.ok) {
      showInlineError("editDateError", await responseError(response, "Failed to update date"))
      return
    }

    const ticket = allData.find(item => item.ticket_number === editingTicketNumber)
    if (ticket) ticket.expected_date = expectedDate
    closeModal("editModal")
    render()
    showToast("Expected date updated", "success")
  } catch (error) {
    console.error(error)
    showInlineError("editDateError", "Network error. Please try again.")
  } finally {
    setSubmitLoading("editDateSubmitBtn", false, "Saving...", "Save")
  }
}

async function deleteTicket(ticketNumber) {
  if (!window.confirm(`Delete ${ticketNumber} and its local updates?`)) return

  try {
    const response = await fetch(`/api/tickets/${encodeURIComponent(ticketNumber)}`, { method: "DELETE" })
    if (!response.ok) throw new Error(await responseError(response, "Failed to delete ticket"))

    allData = allData.filter(ticket => ticket.ticket_number !== ticketNumber)
    updateStats()
    render()
    showToast("Ticket deleted", "success")
  } catch (error) {
    console.error(error)
    showToast(error.message || "Delete failed", "error")
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
  if (sortField === field) {
    sortAsc = !sortAsc
  } else {
    sortField = field
    sortAsc = true
  }
  render()
}

async function exportClosedTickets(startDate, endDate) {
  if (typeof ExcelJS === "undefined") {
    showToast("Export library is unavailable", "error")
    return
  }

  const start = new Date(`${startDate}T00:00:00`)
  const endExclusive = new Date(`${endDate}T00:00:00`)
  endExclusive.setDate(endExclusive.getDate() + 1)
  const filtered = allData.filter(ticket => {
    if (!ticket.ticket_closed_at) return false
    const closedAt = new Date(ticket.ticket_closed_at)
    return closedAt >= start && closedAt < endExclusive
  })

  if (filtered.length === 0) {
    showToast("No closed tickets in this date range", "error")
    return
  }

  try {
    const workbook = new ExcelJS.Workbook()
    const sheet = workbook.addWorksheet("Closed Tickets")
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

    filtered.forEach((ticket, index) => {
      const row = sheet.addRow({
        ticket: ticket.ticket_number,
        summary: ticket.summary,
        system: ticketSystems(ticket),
        reporter: ticket.reporter,
        assignee: ticket.assignee || "—",
        expected: dateOnly(ticket.expected_date),
        closed: formatDateTime(ticket.ticket_closed_at),
        updates: Number(ticket.update_count || 0),
        latestUpdate: formatDateTime(ticket.latest_update_at),
        status: "CLOSED"
      })
      row.eachCell(cell => { cell.alignment = { horizontal: "center", vertical: "middle" } })
      if (index % 2 === 0) {
        row.eachCell(cell => {
          cell.fill = { type: "pattern", pattern: "solid", fgColor: { argb: "FFF8FAFC" } }
        })
      }
    })

    sheet.views = [{ state: "frozen", ySplit: 1 }]
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

async function quickExport(days) {
  const end = today()
  const start = today()
  start.setDate(start.getDate() - days)
  document.getElementById("exportPanel").classList.add("hidden")
  await exportClosedTickets(formatDateLocal(start), formatDateLocal(end))
}

async function confirmExport() {
  const start = document.getElementById("exportStart").value
  const end = document.getElementById("exportEnd").value
  if (!start || !end) {
    showToast("Select both export dates", "error")
    return
  }
  if (start > end) {
    showToast("Start date must be before end date", "error")
    return
  }

  document.getElementById("exportPanel").classList.add("hidden")
  await exportClosedTickets(start, end)
}

document.getElementById("search").addEventListener("input", () => {
  currentPage = 1
  render()
})

document.querySelectorAll("[data-status-filter]").forEach(button => {
  button.addEventListener("click", () => {
    statusFilter = button.dataset.statusFilter
    currentPage = 1
    updateStatusButtons()
    render()
  })
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
  if (currentPage > 1) {
    currentPage -= 1
    render()
  }
})

document.getElementById("nextBtn").addEventListener("click", () => {
  const totalPages = Math.max(1, Math.ceil(getFilteredTickets().length / pageSize))
  if (currentPage < totalPages) {
    currentPage += 1
    render()
  }
})

document.getElementById("tbody").addEventListener("click", event => {
  const button = event.target.closest("button[data-action]")
  if (!button) return

  const ticketNumber = button.dataset.ticket
  switch (button.dataset.action) {
    case "view":
      openTicketDetails(ticketNumber)
      break
    case "updates":
      openTicketDetails(ticketNumber, true)
      break
    case "edit":
      openEditDate(ticketNumber)
      break
    case "delete":
      deleteTicket(ticketNumber)
      break
  }
})

document.getElementById("refreshBtn").addEventListener("click", loadTickets)
document.getElementById("openAddModalBtn").addEventListener("click", openAddModal)
document.getElementById("addTicketForm").addEventListener("submit", addTicket)
document.getElementById("editDateForm").addEventListener("submit", updateExpectedDate)
document.getElementById("updateForm").addEventListener("submit", addTicketUpdate)
document.getElementById("updateContent").addEventListener("input", updateCharacterCounter)

document.querySelectorAll("[data-close-modal]").forEach(button => {
  button.addEventListener("click", () => closeModal(button.dataset.closeModal))
})

document.querySelectorAll("[data-modal]").forEach(modal => {
  modal.addEventListener("click", event => {
    if (event.target === modal) closeModal(modal.id)
  })
})

document.getElementById("exportToggle").addEventListener("click", event => {
  event.stopPropagation()
  document.getElementById("exportPanel").classList.toggle("hidden")
})

document.querySelectorAll("[data-export-days]").forEach(button => {
  button.addEventListener("click", () => quickExport(Number(button.dataset.exportDays)))
})
document.getElementById("exportConfirm").addEventListener("click", confirmExport)

document.addEventListener("click", event => {
  const panel = document.getElementById("exportPanel")
  if (!panel.contains(event.target) && !document.getElementById("exportToggle").contains(event.target)) {
    panel.classList.add("hidden")
  }
})

document.addEventListener("keydown", event => {
  if (event.key !== "Escape") return
  document.getElementById("exportPanel").classList.add("hidden")
  for (const id of ["detailModal", "editModal", "addModal"]) {
    if (!document.getElementById(id).classList.contains("hidden")) {
      closeModal(id)
      break
    }
  }
})

document.getElementById("newExpectedDate").min = todayStr()
document.getElementById("newExpectedDate").value = defaultExpectedDate()
document.getElementById("editExpectedDate").min = todayStr()
document.getElementById("exportEnd").value = todayStr()
const defaultExportStart = today()
defaultExportStart.setDate(defaultExportStart.getDate() - 30)
document.getElementById("exportStart").value = formatDateLocal(defaultExportStart)
updateStatusButtons()
updateCharacterCounter()
loadTickets()
