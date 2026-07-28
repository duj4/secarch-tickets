    // date
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
      const d = today()
      d.setDate(d.getDate() + days)
      return formatDateLocal(d)
    }

    function isPastDate(dateStr) {
      return new Date(dateStr) < today()
    }


    // status
    let allData = []
    let currentPage = 1
    let pageSize = 10
    let sortField = "expected_date"
    let sortAsc = true

    function getStatusBadge(status, closedAt) {
      const map = {
        open: "bg-green-100 text-green-600",
        closed: "bg-gray-200 text-gray-500"
      }

      const cls = map[status] || "bg-gray-200 text-gray-600"

      // 👉 OPEN 直接返回
      if (status === "open") {
        return `
<span class="px-2 py-1 rounded-full text-xs font-semibold ${cls}">
  OPEN
</span>
`
      }

      // 👉 CLOSED 带 tooltip
      return `
<div class="relative group inline-block">
  <span class="px-2 py-1 rounded-full text-xs font-semibold ${cls}">
    CLOSED
  </span>

  <div class="
    absolute bottom-full left-1/2 -translate-x-1/2 mb-1
    hidden group-hover:block
    bg-black text-white text-xs px-2 py-1 rounded
    whitespace-nowrap
    z-10
  ">
    Closed at: ${closedAt ? closedAt.replace("T", " ").slice(0, 16) : "-"}
  </div>
</div>
`
    }

    function getFilteredTickets() {
      const search = document.getElementById("search").value.toLowerCase()
      const statusFilter = document.getElementById("statusFilter").value

      return allData.filter(t => {
        const matchesSearch =
          (t.ticket_number || "").toLowerCase().includes(search) ||
          (t.summary || "").toLowerCase().includes(search)

        const isClosed = !!t.ticket_closed_at
        const matchesStatus =
          statusFilter === "all" ||
          (statusFilter === "open" && !isClosed) ||
          (statusFilter === "closed" && isClosed)

        return matchesSearch && matchesStatus
      })
    }

    // initialization
    function initDateInputs() {
      const input = document.getElementById("newExpectedDate")
      input.min = todayStr()
      input.value = defaultExpectedDate()
    }

    // render
    function render() {

      // ===== FLIP: record old positions =====
      const oldPositions = {}
      document.querySelectorAll("#tbody tr").forEach(tr => {
        oldPositions[tr.dataset.id] = tr.getBoundingClientRect()
      })

      const tbody = document.getElementById("tbody")
      let data = getFilteredTickets()

      // ===== sort =====
      data.sort((a, b) => {
        let v1 = a[sortField] || ""
        let v2 = b[sortField] || ""

        if (sortField === "expected_date") {
          v1 = new Date(v1 || 0)
          v2 = new Date(v2 || 0)
        }

        if (typeof v1 === "string") v1 = v1.toLowerCase()
        if (typeof v2 === "string") v2 = v2.toLowerCase()

        if (v1 > v2) return sortAsc ? 1 : -1
        if (v1 < v2) return sortAsc ? -1 : 1
        return 0
      })

      // ===== pagination =====
      const totalPages = Math.max(1, Math.ceil(data.length / pageSize))
      if (currentPage > totalPages) currentPage = totalPages

      const start = (currentPage - 1) * pageSize
      const pageData = data.slice(start, start + pageSize)

      document.getElementById("pageInfo").textContent =
        `Page ${currentPage} / ${totalPages}`

      document.getElementById("nextBtn").disabled = currentPage >= totalPages
      document.getElementById("prevBtn").disabled = currentPage <= 1

      // ===== UI fade (小优化) =====
      tbody.style.opacity = "0.6"
      tbody.style.transform = "translateY(4px)"

      tbody.innerHTML = ""

      if (pageData.length === 0) {
        tbody.innerHTML = `
        <tr>
          <td colspan="8" class="text-center py-10 text-gray-400">
            <div class="flex flex-col items-center gap-2">
              <div class="text-2xl">📭</div>
              <div class="font-medium">No tickets found</div>
              <div class="text-xs text-gray-400">
                Try adjusting your search or filters
              </div>
            </div>
          </td>
        </tr>
        `
        tbody.style.opacity = "1"
        tbody.style.transform = "translateY(0)"
        return
      }

      pageData.forEach((t, i) => {
        const status = t.ticket_closed_at ? "closed" : "open"
        const ticketUrl = `https://itsm.ai.ms.com.cn/projects/ITSM/queues/issue/${t.ticket_number}`

        const tr = document.createElement("tr")
        tr.dataset.id = t.ticket_number

        tr.className = "border-t transition-all duration-150 hover:bg-gray-200 "

        tr.innerHTML = `
<td class="px-4 py-3 whitespace-nowrap text-blue-600 font-semibold">
  <a href="${ticketUrl}" target="_blank" rel="noopener noreferrer" class="hover:underline">
    ${t.ticket_number}
  </a>
</td>

<td class="px-4 py-3">${t.summary}</td>

<td class="px-4 py-3">${t.cmdb_system_name || ""}</td>

<td class="px-4 py-3 text-center align-middle">${t.reporter}</td>

<td class="px-4 py-3 text-center align-middle">${t.assignee || "-"}</td>

<td class="px-4 py-3 whitespace-nowrap">
  ${t.expected_date?.slice(0, 10)}
</td>

<td class="px-4 py-3">
  ${getStatusBadge(status, t.ticket_closed_at)}
</td>

<td class="px-4 py-3 align-middle">
  <div class="flex items-center gap-2 justify-center">

    <button onclick="viewTicket('${t.ticket_number}')"
      class="text-gray-500 hover:scale-150 transition"
      title="View">
      👁
    </button>

    ${t.ticket_closed_at ?
            `<span class="text-gray-300 cursor-not-allowed"
         title="Closed ticket cannot be edited">✏️</span>`
            :
            `<button onclick="editDate(this, '${t.ticket_number}', '${t.expected_date || ""}')"
         class="text-gray-500 hover:scale-150 transition"
         title="Edit">
         ✏️
       </button>`
          }

    <button onclick="deleteTicket('${t.ticket_number}')"
      class="text-gray-500 hover:scale-150 transition"
      title="Delete">
      🗑
    </button>

  </div>
</td>
`

        tbody.appendChild(tr)
      })

      // ===== sort arrow (带动画) =====
      document.querySelectorAll("th span").forEach(el => el.innerHTML = "")

      const el = document.getElementById(`sort-${sortField}`)
      if (el) {
        el.innerHTML = `
<span class="inline-block transition-transform duration-200"
      style="transform: rotate(${sortAsc ? "0deg" : "180deg"})">
  ▲
</span>
`
      }

      // ===== FLIP animation =====
      requestAnimationFrame(() => {
        document.querySelectorAll("#tbody tr").forEach(tr => {
          const old = oldPositions[tr.dataset.id]
          const now = tr.getBoundingClientRect()

          if (!old) return

          const dy = old.top - now.top

          if (dy !== 0) {
            tr.style.transform = `translateY(${dy}px)`
            tr.style.transition = "none"

            requestAnimationFrame(() => {
              tr.style.transform = ""
              tr.style.transition = "transform 180ms cubic-bezier(0.22, 1, 0.36, 1)"
            })

            // ===== 行变化高亮 =====
            tr.classList.add("bg-blue-50")
            setTimeout(() => {
              tr.classList.remove("bg-blue-50")
            }, 250)
          }
        })

        // ===== UI恢复 =====
        setTimeout(() => {
          tbody.style.opacity = "1"
          tbody.style.transform = "translateY(0)"
        }, 80)
      })
    }

    // inline edit
    function editDate(td, ticketNumber, currentDate) {
      const t = allData.find(x => x.ticket_number === ticketNumber)
      if (t?.ticket_closed_at) {
        alert("Closed ticket cannot be edited")
        return
      }

      const input = document.createElement("input")

      input.type = "date"
      input.value = currentDate?.slice(0, 10)
      input.min = todayStr()

      input.onchange = async () => {
        const newDate = input.value

        if (!newDate) {
          td.innerText = currentDate.slice(0, 10)
          return
        }

        await updateExpectedDate(ticketNumber, newDate)
      }

      td.innerHTML = ""
      td.appendChild(input)

      setTimeout(() => {
        input.showPicker?.()
      })

      input.focus()
    }

    // api
    async function updateExpectedDate(ticketNumber, expectedDate) {
      try {
        await fetch(`/api/tickets/${ticketNumber}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ expected_date: expectedDate })
        })

        showToast("Updated", "success")

        await loadTickets()
      } catch {
        showToast("Update failed", "error")
      }
    }

    async function deleteTicket(ticketNumber) {
      if (!confirm(`Delete ${ticketNumber}?`)) return

      try {
        await fetch(`/api/tickets/${ticketNumber}`, {
          method: "DELETE"
        })

        showToast("Deleted", "success")

        await loadTickets()
      } catch {
        showToast("Delete failed", "error")
      }
    }

    async function addTicket() {
      const ticketNumber = document.getElementById("newTicket").value.trim()
      const expectedDate = document.getElementById("newExpectedDate").value

      if (!ticketNumber || !expectedDate) {
        showToast("Missing input", "error")
        return
      }

      if (!/^ITSM-\d+$/.test(ticketNumber)) {
        showToast("Invalid format (e.g. ITSM-12345)", "error")
        return
      }

      const btn = document.getElementById("addBtn")
      btn.disabled = true
      btn.textContent = "Adding..."

      try {
        const res = await fetch("/api/tickets", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            ticket_number: ticketNumber,
            expected_date: expectedDate
          })
        })

        const data = await res.json().catch(() => ({}))

        if (!res.ok) {
          showToast(data.error || "Add failed", "error")
          return
        }

        showToast("Ticket added", "success")

        document.getElementById("newTicket").value = ""
        document.getElementById("newExpectedDate").value = defaultExpectedDate()

        await loadTickets()

      } catch (err) {
        console.error("network error:", err)
        showToast(data.error || "Network error", "error")
      } finally {
        btn.disabled = false
        btn.textContent = "Add"
      }
    }

    // interaction
    function viewTicket(ticketNumber) {
      const t = allData.find(x => x.ticket_number === ticketNumber)

      if (!t) {
        showToast("Ticket not found", "error")
        return
      }

      const content = `
  <div><b>Ticket:</b> ${t.ticket_number}</div>
  <div><b>System:</b> ${t.cmdb_system_name || "-"}</div>
  <div><b>Reporter:</b> ${t.reporter}</div>
  <div><b>Assignee:</b> ${t.assignee || "-"}</div>
  <div><b>Expected:</b> ${t.expected_date?.slice(0, 10)}</div>
  <div><b>Status:</b> ${t.ticket_closed_at ? "CLOSED" : "OPEN"}</div>
  ${t.ticket_closed_at
          ? `<div><b>Closed At:</b> ${t.ticket_closed_at.replace("T", " ").slice(0, 16)}</div>`
          : ""
        }
  <div class="mt-2">
    <b>Summary:</b>
    <div class="text-gray-600 mt-1">${t.summary}</div>
  </div>
`

      document.getElementById("modalContent").innerHTML = content
      document.getElementById("modal").classList.remove("hidden")
      document.getElementById("modal").classList.add("flex")
    }

    function closeModal() {
      const modal = document.getElementById("modal")
      modal.classList.add("hidden")
      modal.classList.remove("flex")
    }

    function showToast(message, type = "success") {
      const container = document.getElementById("toastContainer")

      const colorMap = {
        success: "bg-green-500",
        error: "bg-red-500",
        info: "bg-gray-500"
      }

      const toast = document.createElement("div")
      toast.className = `
${colorMap[type]}
text-white px-4 py-2 rounded shadow-lg
transform transition-all duration-300
opacity-0 translate-y-2
`

      toast.textContent = message

      container.appendChild(toast)

      setTimeout(() => {
        toast.classList.remove("opacity-0", "translate-y-2")
      }, 10)

      setTimeout(() => {
        toast.classList.add("opacity-0", "translate-y-2")
        setTimeout(() => toast.remove(), 300)
      }, 2000)
    }

    function prevPage() {
      if (currentPage > 1) {
        currentPage--
        render()
      }
    }

    function nextPage() {
      const totalPages = Math.max(1, Math.ceil(getFilteredTickets().length / pageSize))

      if (currentPage < totalPages) {
        currentPage++
        render()
      }
    }

    function sortBy(field) {
      if (sortField === field) {
        sortAsc = !sortAsc
      } else {
        sortField = field
        sortAsc = true
      }
      render()
    }

    async function loadTickets() {
      const tbody = document.getElementById("tbody")
      tbody.innerHTML = `
${Array.from({ length: 5 }).map(() => `
<tr class="border-t">
  <td class="px-4 py-3">
    <div class="h-4 w-24 bg-gray-200 rounded animate-pulse"></div>
  </td>

  <td class="px-4 py-3">
    <div class="space-y-2">
      <div class="h-4 w-full bg-gray-200 rounded animate-pulse"></div>
      <div class="h-4 w-3/4 bg-gray-200 rounded animate-pulse"></div>
    </div>
  </td>

  <td class="px-4 py-3">
    <div class="h-4 w-40 bg-gray-200 rounded animate-pulse"></div>
  </td>

  <td class="px-4 py-3 text-center">
    <div class="h-4 w-16 bg-gray-200 rounded mx-auto animate-pulse"></div>
  </td>

  <td class="px-4 py-3 text-center">
    <div class="h-4 w-16 bg-gray-200 rounded mx-auto animate-pulse"></div>
  </td>

  <td class="px-4 py-3">
    <div class="h-4 w-24 bg-gray-200 rounded animate-pulse"></div>
  </td>

  <td class="px-4 py-3">
    <div class="h-6 w-16 bg-gray-200 rounded-full animate-pulse"></div>
  </td>

  <td class="px-4 py-3">
    <div class="flex justify-center gap-3">
      <div class="h-5 w-5 bg-gray-200 rounded-full animate-pulse"></div>
      <div class="h-5 w-5 bg-gray-200 rounded-full animate-pulse"></div>
      <div class="h-5 w-5 bg-gray-200 rounded-full animate-pulse"></div>
    </div>
  </td>
</tr>
`).join("")}`

      try {
        const res = await fetch("/api/tickets")
        const resp = await res.json()

        allData = resp.tickets || []
        currentPage = 1

        render()
      } catch (e) {
        tbody.innerHTML = `<tr><td colspan="7">Failed to load</td></tr>`
      }
    }

    function toggleExport() {
      const panel = document.getElementById("exportPanel")
      panel.classList.toggle("hidden")
    }

    function quickExport(days) {
      const end = today()
      const start = new Date()
      start.setDate(end.getDate() - days)

      const startStr = formatDateLocal(start)
      const endStr = formatDateLocal(end)

      exportClosedTickets(startStr, endStr)

      showToast(`Export last ${days} days`, "success")
      document.getElementById("exportPanel").classList.add("hidden")
    }

    function confirmExport() {
      const start = document.getElementById("exportStart").value
      const end = document.getElementById("exportEnd").value

      if (!start || !end) {
        showToast("Please select date range", "error")
        return
      }

      exportClosedTickets(start, end)

      showToast("Export success", "success")
      document.getElementById("exportPanel").classList.add("hidden")
    }

    async function exportClosedTickets(startDate, endDate) {
      const filtered = allData.filter(t => {
        if (!t.ticket_closed_at) return false
        const closed = new Date(t.ticket_closed_at)
        return closed >= new Date(startDate) && closed <= new Date(endDate)
      })

      if (filtered.length === 0) {
        showToast("No data", "error")
        return
      }

      const wb = new ExcelJS.Workbook()
      const ws = wb.addWorksheet("Closed Tickets")

      // ===== header =====
      ws.columns = [
        { header: "Ticket", key: "ticket", width: 15 },
        { header: "Summary", key: "summary", width: 50 },
        { header: "System", key: "system", width: 30 },
        { header: "Reporter", key: "reporter", width: 15 },
        { header: "Assignee", key: "assignee", width: 15 },
        { header: "Expected", key: "expected", width: 15 },
        { header: "Closed At", key: "closed", width: 25 },
        { header: "Status", key: "status", width: 12 }
      ]

      // ===== header style =====
      ws.getRow(1).eachCell(cell => {
        cell.font = { bold: true }
        cell.alignment = { horizontal: "center" }
        cell.fill = {
          type: "pattern",
          pattern: "solid",
          fgColor: { argb: "FFE5E7EB" }
        }
      })

      // ===== data =====
      filtered.forEach((t, i) => {
        const row = ws.addRow({
          ticket: t.ticket_number,
          summary: t.summary,
          system: t.cmdb_system_name || "",
          reporter: t.reporter,
          assignee: t.assignee || "-",
          expected: t.expected_date?.slice(0, 10),
          closed: formatDateTime(t.ticket_closed_at),
          status: "CLOSED"
        })

        row.eachCell(cell => {
          cell.alignment = { horizontal: "center", vertical: "middle" }
        })

        // zebra
        if (i % 2 === 0) {
          row.eachCell(cell => {
            cell.fill = {
              type: "pattern",
              pattern: "solid",
              fgColor: { argb: "FFF9FAFB" }
            }
          })
        }

        // status color
        row.getCell("status").fill = {
          type: "pattern",
          pattern: "solid",
          fgColor: { argb: "FFE5E7EB" }
        }
      })

      // freeze
      ws.views = [{ state: "frozen", ySplit: 1 }]

      // download
      const buffer = await wb.xlsx.writeBuffer()
      const blob = new Blob([buffer])
      const url = URL.createObjectURL(blob)

      const a = document.createElement("a")
      a.href = url
      const fileName = `closed_ai_secarch_tickets_${startDate}_to_${endDate}.xlsx`
      a.download = fileName
      a.click()
    }

    function formatDateTime(str) {
      if (!str) return ""
      const d = new Date(str)
      return `${d.toLocaleDateString()} ${d.toLocaleTimeString()}`
    }

    // listen
    document.getElementById("search").addEventListener("input", () => {
      currentPage = 1
      render()
    })

    document.getElementById("statusFilter").addEventListener("change", () => {
      currentPage = 1
      render()
    })

    document.getElementById("pageSize").addEventListener("change", e => {
      pageSize = parseInt(e.target.value)
      currentPage = 1
      render()
    })

    document.getElementById("newTicket").addEventListener("keypress", e => {
      if (e.key == "Enter") {
        document.getElementById("newExpectedDate").focus()
      }
    })

    document.getElementById("newExpectedDate").addEventListener("keypress", e => {
      if (e.key == "Enter") {
        addTicket()
      }
    })

    document.getElementById("modal").addEventListener("click", e => {
      if (e.target.id === "modal") {
        closeModal()
      }
    })

    document.addEventListener("keydown", e => {
      if (e.key === "Escape") closeModal()
    })

    document.addEventListener("click", (e) => {
      const panel = document.getElementById("exportPanel")
      const btn = e.target.closest("button[onclick='toggleExport()']")

      if (!panel) return

      if (!panel.contains(e.target) && !btn) {
        panel.classList.add("hidden")
      }
    })

    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        document.getElementById("exportPanel")?.classList.add("hidden")
      }
    })

    // init
    initDateInputs()
    loadTickets()
