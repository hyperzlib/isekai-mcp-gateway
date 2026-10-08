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
