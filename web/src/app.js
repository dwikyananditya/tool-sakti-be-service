import Alpine from "alpinejs";
import Dexie from "dexie";
import {
  createIcons,
  Download,
  LoaderCircle,
  TriangleAlert,
  Sun,
  Moon,
  ArrowLeft,
  Copy,
} from "lucide";

window.Alpine = Alpine;
Alpine.start();

const icons = { Download, LoaderCircle, TriangleAlert, Sun, Moon, ArrowLeft, Copy };
const drawIcons = () => createIcons({ icons });

const db = new Dexie("tool-sakti-be-service");
db.version(1).stores({ results: "id, createdAt" });

function esc(s) {
  return String(s ?? "").replace(
    /[&<>"']/g,
    (m) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[m],
  );
}

function spinner(on) {
  const el = document.getElementById("spinner");
  if (el) el.style.display = on ? "grid" : "none";
}


function wireScanForm() {
  const form = document.getElementById("scan-form");
  if (!form) return;
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const btn = form.querySelector("button[type=submit]");
    if (btn) btn.disabled = true;
    spinner(true);
    try {
      const res = await fetch("/scan", { method: "POST", body: new FormData(form) });
      if (res.redirected) {
        location.href = res.url;
        return;
      }
      const data = await res.json();
      if (!res.ok) {
        showFormError(data.error || "Gagal scan.");
        return;
      }
      const id = crypto.randomUUID().slice(0, 8);
      await db.results.put({ id, createdAt: Date.now(), ...data });
      location.href = "/r/" + id;
    } catch {
      showFormError("Gagal menghubungi server.");
    } finally {
      spinner(false);
      if (btn) btn.disabled = false;
    }
  });
}

function showFormError(msg) {
  const box = document.getElementById("form-error");
  if (!box) {
    alert(msg);
    return;
  }
  box.querySelector("[data-msg]").textContent = msg;
  box.classList.remove("hidden");
  drawIcons();
}


async function renderResultPage() {
  const root = document.getElementById("result");
  if (!root) return;
  const m = location.pathname.match(/^\/r\/([A-Za-z0-9-]+)$/);
  const rec = m ? await db.results.get(m[1]) : null;
  if (!rec) {
    root.innerHTML = notFoundHTML();
    drawIcons();
    return;
  }
  root.innerHTML = resultHTML(rec);
  drawIcons();
  document.getElementById("download")?.addEventListener("click", () => downloadXlsx(rec));
}

function confidenceVariant(c) {
  if (c === "HIGH") return "primary";
  if (c === "MEDIUM") return "secondary";
  return "outline";
}

function cellOrDash(v) {
  return v
    ? esc(v)
    : '<span class="text-muted-foreground">—</span>';
}

function resultHTML(rec) {
  const rows = rec.rows || [];
  const body = rows
    .map(
      (r) => `
      <tr class="${r.needsReview ? "bg-amber-500/10" : ""}">
        <td class="truncate font-medium">${esc(r.assignee)}</td>
        <td class="truncate">
          <a href="${esc(r.url)}" target="_blank" class="underline-offset-4 hover:underline" title="${esc(r.title)}">${esc(r.title)}</a>
        </td>
        <td class="truncate">${esc(r.date)}</td>
        <td class="truncate"><span class="badge" data-variant="outline">${esc(r.status)}</span></td>
        <td class="truncate font-mono text-xs">${cellOrDash(r.start)}</td>
        <td class="truncate font-mono text-xs">${cellOrDash(r.end)}</td>
        <td class="text-right font-mono tabular-nums">${cellOrDash(r.hours)}</td>
        <td class="truncate"><span class="badge" data-variant="${confidenceVariant(r.confidence)}" title="${esc(r.rule)}">${esc(r.confidence)}</span></td>
      </tr>`,
    )
    .join("");

  const reviewAlert = rec.review
    ? `<div class="alert mb-4" data-variant="default">
         <i data-lucide="triangle-alert"></i>
         <section><p>${rec.review} baris tidak menemukan End valid dari aktivitas GitHub — cek sheet Diagnostic di workbook.</p></section>
       </div>`
    : "";

  const errStat = rec.scanErrors
    ? `<div><div class="text-xs text-muted-foreground">Error scan</div><div class="text-2xl font-semibold tabular-nums text-destructive">${rec.scanErrors}</div></div>`
    : "";

  return `
    <div class="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div class="flex gap-6">
        <div><div class="text-xs text-muted-foreground">Baris</div><div class="text-2xl font-semibold tabular-nums">${rec.total}</div></div>
        <div><div class="text-xs text-muted-foreground">Perlu review</div><div class="text-2xl font-semibold tabular-nums ${rec.review ? "text-amber-500" : "text-emerald-500"}">${rec.review}</div></div>
        ${errStat}
      </div>
      <div class="flex gap-2">
        <a href="/" class="btn" data-variant="outline"><i data-lucide="arrow-left" class="size-4"></i> Scan lagi</a>
        <button type="button" id="download" class="btn"><i data-lucide="download" class="size-4"></i> Download .xlsx</button>
      </div>
    </div>
    ${reviewAlert}
    <div class="table-container">
      <table class="table w-full table-fixed">
        <colgroup>
          <col class="w-24" /><col /><col class="w-28" /><col class="w-36" />
          <col class="w-40" /><col class="w-40" /><col class="w-16" /><col class="w-28" />
        </colgroup>
        <thead><tr>
          <th>Assignee</th><th>Tiket</th><th>Tanggal</th><th>Status</th>
          <th>Start</th><th>End</th><th class="text-right">Jam</th><th>Confidence</th>
        </tr></thead>
        <tbody>${body}</tbody>
      </table>
    </div>`;
}

function notFoundHTML() {
  return `
    <div class="mx-auto max-w-md py-16 text-center">
      <p class="text-muted-foreground">Hasil tidak ditemukan di browser ini.</p>
      <a href="/" class="btn mt-4" data-variant="outline"><i data-lucide="arrow-left" class="size-4"></i> Scan lagi</a>
    </div>`;
}

function downloadXlsx(rec) {
  const bin = atob(rec.xlsx);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  const blob = new Blob([bytes], {
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = rec.fileName || "KPI.xlsx";
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 5000);
}


drawIcons();
wireScanForm();
renderResultPage();
