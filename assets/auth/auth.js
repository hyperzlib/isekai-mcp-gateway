"use strict";

const countdown = document.querySelector("[data-countdown]");
if (countdown) {
  let seconds = Number(countdown.dataset.countdown);
  const label = countdown.textContent;
  const update = () => {
    countdown.disabled = seconds > 0;
    countdown.textContent = seconds > 0 ? `${label}（${seconds} 秒）` : label;
  };
  update();
  const timer = setInterval(() => {
    seconds = Math.max(0, seconds - 1);
    update();
    if (seconds === 0) clearInterval(timer);
  }, 1000);
}

const form = document.querySelector("#consent");
if (form) {
  const inputs = new Map([...form.querySelectorAll('input[name="scope"]')].map(input => [input.value, input]));
  const set = (name, checked) => { if (inputs.has(name)) inputs.get(name).checked = checked; };
  form.addEventListener("change", event => {
    const input = event.target;
    if (input.name !== "scope") return;
    if (input.checked && input.value.startsWith("wiki:") && input.value !== "wiki:read") set("wiki:read", true);
    if (input.checked && input.value === "wiki:create") set("wiki:edit", true);
    if (!input.checked && input.value === "wiki:edit") set("wiki:create", false);
    if (!input.checked && input.value === "wiki:read") {
      for (const [name, checkbox] of inputs) if (name.startsWith("wiki:")) checkbox.checked = false;
    }
  });
}

function openDialog(dialog) {
  if (!dialog || dialog.open) return;
  dialog.classList.remove("is-closing");
  dialog.showModal();
}
function closeDialog(dialog) {
  if (!dialog?.open || dialog.classList.contains("is-closing")) return;
  if (matchMedia("(prefers-reduced-motion: reduce)").matches) { dialog.close(); return; }
  dialog.classList.add("is-closing");
  let finished = false;
  const finish = event => { if (event && (event.target !== dialog || event.pseudoElement || event.animationName !== "modal-out")) return; if (finished) return; finished = true; dialog.removeEventListener("animationend", finish); if (dialog.open) dialog.close(); dialog.classList.remove("is-closing"); };
  dialog.addEventListener("animationend", finish);
  setTimeout(finish, 250);
}
for (const trigger of document.querySelectorAll("[data-open-dialog]")) {
  trigger.addEventListener("click", () => openDialog(document.getElementById(trigger.dataset.openDialog)));
}
for (const trigger of document.querySelectorAll("[data-close-dialog]")) {
  trigger.addEventListener("click", () => closeDialog(trigger.closest("dialog")));
}
for (const dialog of document.querySelectorAll("dialog.modal")) {
  dialog.addEventListener("cancel", event => { event.preventDefault(); closeDialog(dialog); });
}

document.addEventListener("alpine:init", () => {
  Alpine.data("apiKeyForm", () => ({
    nameError: "", expiryError: "",
    init() { this.$el.noValidate = true; },
    validate(event) {
      const form = this.$el;
      const name = form.elements.name.value.trim();
      const expiry = form.querySelector('input[name="expiry"]:checked');
      const date = form.elements.customDate;
      const max = Number(form.dataset.maxTtl);
      this.nameError = !name ? "请输入名称。" : /[\r\n]/.test(name) || new TextEncoder().encode(name).length > 64 ? "名称不能包含换行，且不能超过 64 字节。" : "";
      this.expiryError = "";
      if (!expiry) this.expiryError = "请选择有效期。";
      else if (expiry.value === "custom") {
        const end = Date.parse(date.value + "T00:00:00Z") + 86400000;
        if (!date.value || !Number.isFinite(end) || end <= Date.now() || Math.floor((end - Date.now()) / 1000) > max) this.expiryError = "请选择有效范围内的日期。";
      }
      if (this.nameError || this.expiryError) {
        event.preventDefault();
        (this.nameError ? form.elements.name : expiry?.value === "custom" ? date : form.querySelector('input[name="expiry"]:not(:disabled)')).focus();
      }
    }
  }));
});

const confirmDialog = document.getElementById("confirm-removal");
let pendingRemoval = null;
for (const form of document.querySelectorAll("form[data-confirm-name]")) {
  let confirmed = false;
  form.addEventListener("submit", event => {
    if (confirmed) { confirmed = false; return; }
    event.preventDefault();
    pendingRemoval = { form, submitter: event.submitter, name: form.dataset.confirmName || "该项目" };
    confirmDialog.querySelector("#confirm-title").textContent = form.dataset.confirmTitle;
    confirmDialog.querySelector(".confirm-name").textContent = pendingRemoval.name;
    confirmDialog.querySelector("#confirm-message").textContent = form.dataset.confirmMessage;
    openDialog(confirmDialog);
  });
  form.confirmRemoval = () => { confirmed = true; form.requestSubmit(pendingRemoval.submitter || undefined); };
}
confirmDialog?.querySelector("[data-confirm-submit]").addEventListener("click", () => {
  if (!pendingRemoval) return;
  try {
    const marker = Array.from(crypto.getRandomValues(new Uint8Array(16)), n => n.toString(16).padStart(2, "0")).join("");
    sessionStorage.setItem("removal:" + marker, pendingRemoval.name);
    const input = document.createElement("input"); input.type = "hidden"; input.name = "notice_id"; input.value = marker;
    pendingRemoval.form.append(input);
  } catch { /* Submission still works when storage is unavailable. */ }
  pendingRemoval.form.confirmRemoval();
});
const notice = document.querySelector(".removal-notice");
if (notice) {
  const url = new URL(location.href);
  const marker = url.searchParams.get("removed");
  if (marker && /^[0-9a-f]{32}$/.test(marker)) {
    try {
      const name = sessionStorage.getItem("removal:" + marker);
      sessionStorage.removeItem("removal:" + marker);
      if (name) { notice.querySelector("span").textContent = "已删除 " + name; notice.hidden = false; }
    } catch { /* Storage can be disabled. */ }
    url.searchParams.delete("removed");
    history.replaceState(history.state, "", url.pathname + url.search + url.hash);
  }
  notice.querySelector("button").addEventListener("click", () => { notice.hidden = true; });
}

for (const menu of document.querySelectorAll("[data-account-menu]")) {
  document.addEventListener("click", event => { if (!menu.contains(event.target)) menu.open = false; });
  menu.addEventListener("keydown", event => {
    if (event.key === "Escape" && menu.open) { menu.open = false; menu.querySelector("summary").focus(); }
  });
}

for (const button of document.querySelectorAll("[data-copy-key]")) {
  button.addEventListener("click", async () => {
    const key = document.getElementById(button.dataset.copyKey);
    const status = button.closest(".key-result").querySelector(".key-copy-status");
    try {
      await navigator.clipboard.writeText(key.textContent);
      status.textContent = "已复制到剪贴板";
    } catch {
      status.textContent = "复制失败，请手动选择并复制密钥。";
    }
  });
}
