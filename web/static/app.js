// Kitchen Garden Planner — vanilla JS front end for the Go meal-planner API.
// No build step, no external deps: keeps it free and happy on a Raspberry Pi.

const SPRIGS = ["🌿", "🌾", "🌻", "🍅", "🥕", "🫛", "🌼", "🧄", "🍋"];

// --- state ---
let recipes = [];          // from /api/recipes
const plan = [];           // [{recipe, servings}]
let preview = null;        // last /api/plan/preview result
const lineQty = {};        // name -> overridden package count
const stapleChoice = {};   // name -> {action:'yes'|'no'|'snooze', until:'YYYY-MM-DD'}

// --- boot ---
loadRecipes();
document.getElementById("clear-plan").addEventListener("click", clearPlan);
document.getElementById("add-to-cart").addEventListener("click", () => submitCart({}));
document.getElementById("resolve-cancel").addEventListener("click", closeModal);

async function loadRecipes() {
  try {
    const res = await fetch("/api/recipes");
    if (!res.ok) throw new Error(await errText(res));
    recipes = await res.json();
    renderRecipes();
  } catch (e) {
    document.getElementById("recipe-grid").innerHTML =
      `<p class="empty">Couldn't load recipes: ${escapeHtml(e.message)}</p>`;
  }
}

function renderRecipes() {
  const grid = document.getElementById("recipe-grid");
  if (!recipes.length) {
    grid.innerHTML = `<p class="empty">No recipes yet — add one with <code>groceries recipe add</code>. 🌱</p>`;
    return;
  }
  grid.innerHTML = "";
  recipes.forEach((r, i) => {
    const base = r.servings && r.servings > 0 ? r.servings : 1;
    const ingNames = (r.ingredients || []).map((x) => x.name).filter(Boolean);
    const card = document.createElement("div");
    card.className = "recipe-card";
    card.innerHTML = `
      <span class="corner-sprig">${SPRIGS[i % SPRIGS.length]}</span>
      <h3>${escapeHtml(r.name)}</h3>
      <p class="recipe-meta">${(r.ingredients || []).length} ingredients${
        r.servings ? ` · serves ${r.servings}` : ""
      }</p>
      <p class="recipe-ings">${escapeHtml(ingNames.slice(0, 4).join(", "))}${
        ingNames.length > 4 ? "…" : ""
      }</p>
      <div class="card-controls">
        <div class="servings-stepper" data-base="${base}">
          <button type="button" class="serv-minus" aria-label="fewer servings">−</button>
          <span class="count"><span class="serv-val">${base}</span> <small>servings</small></span>
          <button type="button" class="serv-plus" aria-label="more servings">+</button>
        </div>
        <button type="button" class="add-btn">Add</button>
      </div>`;

    const valEl = card.querySelector(".serv-val");
    card.querySelector(".serv-minus").addEventListener("click", () => {
      let v = parseInt(valEl.textContent, 10);
      if (v > 1) valEl.textContent = v - 1;
    });
    card.querySelector(".serv-plus").addEventListener("click", () => {
      valEl.textContent = parseInt(valEl.textContent, 10) + 1;
    });
    card.querySelector(".add-btn").addEventListener("click", () => {
      addToPlan(r.name, parseInt(valEl.textContent, 10));
    });
    grid.appendChild(card);
  });
}

function addToPlan(recipe, servings) {
  const existing = plan.find((p) => p.recipe === recipe);
  if (existing) existing.servings = servings;
  else plan.push({ recipe, servings });
  refreshPreview();
}

function removeFromPlan(recipe) {
  const i = plan.findIndex((p) => p.recipe === recipe);
  if (i >= 0) plan.splice(i, 1);
  refreshPreview();
}

function clearPlan() {
  plan.length = 0;
  for (const k in lineQty) delete lineQty[k];
  for (const k in stapleChoice) delete stapleChoice[k];
  refreshPreview();
}

async function refreshPreview() {
  renderPlanRecipes();
  const cartSection = document.getElementById("cart-section");
  if (!plan.length) {
    preview = null;
    cartSection.hidden = true;
    document.getElementById("clear-plan").hidden = true;
    return;
  }
  document.getElementById("clear-plan").hidden = false;
  try {
    const res = await fetch("/api/plan/preview", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ entries: plan }),
    });
    if (!res.ok) throw new Error(await errText(res));
    preview = await res.json();
    renderCart();
    cartSection.hidden = false;
  } catch (e) {
    setStatus("err", "Preview failed: " + e.message);
    cartSection.hidden = false;
  }
}

function renderPlanRecipes() {
  const box = document.getElementById("plan-recipes");
  if (!plan.length) {
    box.innerHTML = `<p class="empty">No meals planned yet. Add a recipe card to begin. 🧺</p>`;
    return;
  }
  box.innerHTML = "";
  plan.forEach((p) => {
    const row = document.createElement("div");
    row.className = "plan-recipe";
    row.innerHTML = `
      <span class="pr-name">${escapeHtml(p.recipe)}</span>
      <span class="pr-serv">${p.servings} servings</span>
      <button class="pr-remove" title="remove" aria-label="remove ${escapeHtml(
        p.recipe
      )}">×</button>`;
    row.querySelector(".pr-remove").addEventListener("click", () => removeFromPlan(p.recipe));
    box.appendChild(row);
  });
}

function renderCart() {
  // Auto-add lines
  const linesEl = document.getElementById("cart-lines");
  linesEl.innerHTML = "";
  (preview.lines || []).forEach((line) => {
    const qty = lineQty[line.name] != null ? lineQty[line.name] : line.packages;
    const li = document.createElement("li");
    li.className = "cart-line";
    li.innerHTML = `
      <span class="cl-name">${escapeHtml(line.name)}
        ${line.scaledQty ? `<span class="cl-from">≈ ${escapeHtml(line.scaledQty)}</span>` : ""}
        <span class="cl-from">${escapeHtml(line.from || "")}</span>
        ${line.hasPreset ? "" : `<span class="cl-nopreset">first time — I'll ask which product</span>`}
      </span>
      <input class="qty-input" type="number" min="1" value="${qty}" aria-label="quantity for ${escapeHtml(
        line.name
      )}">`;
    li.querySelector(".qty-input").addEventListener("change", (ev) => {
      const v = parseInt(ev.target.value, 10);
      lineQty[line.name] = v > 0 ? v : 1;
    });
    linesEl.appendChild(li);
  });

  // Staples
  const staplesBlock = document.getElementById("staples-block");
  const staplesEl = document.getElementById("staples");
  staplesEl.innerHTML = "";
  const staples = preview.staples || [];
  staplesBlock.hidden = staples.length === 0;
  staples.forEach((st, idx) => {
    if (!stapleChoice[st.name]) stapleChoice[st.name] = { action: "no", until: defaultSnoozeDate() };
    const choice = stapleChoice[st.name];
    const li = document.createElement("li");
    li.className = "staple";
    const grp = "st-" + idx;
    li.innerHTML = `
      <div><span class="st-name">${escapeHtml(st.name)}</span>
        <span class="st-from">${escapeHtml(st.from || "")}</span></div>
      <div class="staple-choices">
        ${radioChip(grp, "yes", "Yes, add", choice.action === "yes")}
        ${radioChip(grp, "no", "No", choice.action === "no")}
        ${radioChip(grp, "snooze", "Snooze", choice.action === "snooze")}
      </div>
      <div class="snooze-date" ${choice.action === "snooze" ? "" : "hidden"}>
        don't ask again until
        <input type="date" value="${choice.until}">
      </div>`;
    li.querySelectorAll(`input[name="${grp}"]`).forEach((radio) => {
      radio.addEventListener("change", () => {
        choice.action = radio.value;
        li.querySelector(".snooze-date").hidden = radio.value !== "snooze";
      });
    });
    li.querySelector('input[type="date"]').addEventListener("change", (ev) => {
      choice.until = ev.target.value;
    });
    staplesEl.appendChild(li);
  });

  // Snoozed note
  const note = document.getElementById("snoozed-note");
  const snoozed = preview.snoozed || [];
  if (snoozed.length) {
    note.hidden = false;
    note.textContent = `💤 Skipping snoozed staples: ${snoozed.join(", ")}.`;
  } else {
    note.hidden = true;
  }

  clearStatus();
}

function radioChip(group, value, label, checked) {
  const id = `${group}-${value}`;
  return `<span class="chip-radio ${value}">
    <input type="radio" id="${id}" name="${group}" value="${value}" ${checked ? "checked" : ""}>
    <label for="${id}">${label}</label>
  </span>`;
}

// --- cart submit + disambiguation ---

async function submitCart(chosen) {
  if (!preview) return;
  const lines = [];

  (preview.lines || []).forEach((line) => {
    const qty = lineQty[line.name] != null ? lineQty[line.name] : line.packages;
    lines.push({ Name: line.name, Quantity: qty });
  });

  const snoozes = [];
  (preview.staples || []).forEach((st) => {
    const c = stapleChoice[st.name] || { action: "no" };
    if (c.action === "yes") {
      lines.push({ Name: st.name, Quantity: st.packages || 1 });
    } else if (c.action === "snooze" && c.until) {
      snoozes.push({ name: st.name, until: c.until });
    }
  });

  if (!lines.length) {
    setStatus("err", "Nothing selected to add.");
    return;
  }

  setStatus("busy", "Filling your cart… 🌾");
  setButtonsDisabled(true);
  try {
    const res = await fetch("/api/cart", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ lines, snoozes, chosen }),
    });
    if (!res.ok) throw new Error(await errText(res));
    const result = await res.json();
    if (result.unresolved && result.unresolved.length) {
      openModal(result.unresolved, chosen);
      setStatus("busy", "A few items need a one-time choice…");
    } else {
      closeModal();
      const n = (result.added || []).length;
      setStatus("ok", `🌸 Added ${n} item${n === 1 ? "" : "s"} to your Kroger cart!`);
    }
  } catch (e) {
    setStatus("err", "Couldn't add: " + e.message);
  } finally {
    setButtonsDisabled(false);
  }
}

function openModal(unresolved, priorChosen) {
  const list = document.getElementById("resolve-list");
  list.innerHTML = "";
  unresolved.forEach((u, idx) => {
    const item = document.createElement("div");
    item.className = "resolve-item";
    item.dataset.name = u.name;
    if (u.candidates && u.candidates.length) {
      const opts = u.candidates
        .map((c, i) => `<option value="${escapeHtml(c.upc)}">${escapeHtml(c.display)}</option>`)
        .join("");
      item.innerHTML = `
        <div class="ri-name">${escapeHtml(u.name)}</div>
        <select data-role="pick">${opts}</select>`;
    } else {
      item.innerHTML = `
        <div class="ri-name">${escapeHtml(u.name)}</div>
        <div class="ri-note">${escapeHtml(u.note || "no matches found")}</div>
        <label class="skip"><input type="checkbox" data-role="skip" checked> skip this item for now</label>`;
    }
    list.appendChild(item);
  });

  const confirmBtn = document.getElementById("resolve-confirm");
  confirmBtn.onclick = () => {
    const chosen = { ...priorChosen };
    const skip = new Set();
    list.querySelectorAll(".resolve-item").forEach((item) => {
      const name = item.dataset.name;
      const pick = item.querySelector('[data-role="pick"]');
      const skipBox = item.querySelector('[data-role="skip"]');
      if (pick) chosen[name] = pick.value;
      else if (skipBox && skipBox.checked) skip.add(name);
    });
    // Drop skipped items from the plan's lines so the cart can go through.
    if (skip.size) {
      preview.lines = (preview.lines || []).filter((l) => !skip.has(l.name));
      (preview.staples || []).forEach((st) => {
        if (skip.has(st.name) && stapleChoice[st.name]) stapleChoice[st.name].action = "no";
      });
    }
    closeModal();
    submitCart(chosen);
  };
  document.getElementById("resolve-overlay").hidden = false;
}

function closeModal() {
  document.getElementById("resolve-overlay").hidden = true;
}

// --- small helpers ---

function setStatus(kind, msg) {
  const el = document.getElementById("status");
  el.hidden = false;
  el.className = "status " + kind;
  el.textContent = msg;
}
function clearStatus() {
  const el = document.getElementById("status");
  el.hidden = true;
  el.textContent = "";
}
function setButtonsDisabled(d) {
  document.getElementById("add-to-cart").disabled = d;
}
function defaultSnoozeDate() {
  const d = new Date();
  d.setMonth(d.getMonth() + 1);
  return d.toISOString().slice(0, 10);
}
async function errText(res) {
  try {
    const j = await res.json();
    return j.error || res.statusText;
  } catch {
    return res.statusText || "request failed";
  }
}
function escapeHtml(s) {
  return String(s == null ? "" : s)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
