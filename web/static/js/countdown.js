// Ticks the countdown, flips the page when the day starts or ends, and throws
// confetti. All times come from the server (data-* on .stage), corrected for
// the local clock's skew, so a wrong device clock still counts correctly.
(() => {
    "use strict";

    const stage = document.querySelector(".stage[data-start]");
    if (!stage) return;

    const start = Date.parse(stage.dataset.start);
    const end = Date.parse(stage.dataset.end);
    const skew = Date.parse(stage.dataset.now) - Date.now();
    const today = stage.dataset.today === "true";
    const open = stage.dataset.open === "true";
    const until = stage.dataset.until ? Date.parse(stage.dataset.until) : 0;
    const now = () => Date.now() + skew;
    const calm = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    const HOUR = 3600e3;
    const MINUTE = 60e3;
    const DAY = 24 * HOUR;

    const tiles = {};
    document.querySelectorAll("[data-unit]").forEach((el) => {
        tiles[el.dataset.unit] = el;
    });
    const endsEl = document.querySelector("[data-ends]");

    const pad = (n) => String(n).padStart(2, "0");
    const plural = (n, word) => `${n} ${word}${n === 1 ? "" : "s"}`;

    function split(ms) {
        return {
            days: Math.floor(ms / DAY),
            hours: Math.floor((ms % DAY) / HOUR),
            minutes: Math.floor((ms % HOUR) / MINUTE),
            seconds: Math.floor((ms % MINUTE) / 1000),
        };
    }

    function setTile(unit, value) {
        const el = tiles[unit];
        if (!el) return;
        const text = unit === "days" ? String(value) : pad(value);
        if (el.textContent === text) return;
        el.textContent = text;
        if (!calm) {
            el.classList.remove("tick");
            void el.offsetWidth; // restart the animation
            el.classList.add("tick");
        }
    }

    // When the target passes (or the I.O.U. should hide), ask the server what
    // state we are in now and
    // reload only once it agrees, so clock drift never causes a reload loop.
    async function settle() {
        try {
            const res = await fetch("/api/countdown", { cache: "no-store" });
            if (res.ok) {
                const m = await res.json();
                if (m.today !== today || m.open !== open || Date.parse(m.start) !== start) {
                    window.location.reload();
                    return;
                }
            }
        } catch {
            // Offline or restarting: try again shortly.
        }
        setTimeout(settle, 5000);
    }

    function tick() {
        const t = now();
        const left = (today ? end : start) - t;
        if (left <= 0 || (until && t >= until)) {
            settle();
            return;
        }

        const parts = split(left);
        if (today) {
            if (endsEl) {
                endsEl.textContent =
                    parts.hours > 0 ? plural(parts.hours, "hour") : plural(Math.max(parts.minutes, 1), "minute");
            }
        } else {
            setTile("days", parts.days);
            setTile("hours", parts.hours);
            setTile("minutes", parts.minutes);
            setTile("seconds", parts.seconds);
        }
        // Re-align on the next whole second.
        setTimeout(tick, 1000 - (t % 1000) + 10);
    }

    tick();

    // Confetti
    const canvas = document.querySelector(".confetti");
    const ctx = canvas && canvas.getContext("2d");
    const styles = getComputedStyle(document.documentElement);
    const colors = ["--rose", "--gold", "--blush", "--butter"];
    if (document.body.classList.contains("spooky")) colors.push("--pumpkin");
    const palette = colors.map((v) => styles.getPropertyValue(v).trim());
    let pieces = [];
    let frame = 0;

    function resize() {
        const dpr = window.devicePixelRatio || 1;
        canvas.width = innerWidth * dpr;
        canvas.height = innerHeight * dpr;
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    function burst(x, y, count, spread) {
        if (!ctx || calm) return;
        for (let i = 0; i < count; i++) {
            const angle = -Math.PI / 2 + (Math.random() - 0.5) * spread;
            const speed = 4 + Math.random() * 7;
            pieces.push({
                x,
                y,
                vx: Math.cos(angle) * speed,
                vy: Math.sin(angle) * speed,
                size: 5 + Math.random() * 6,
                rot: Math.random() * Math.PI,
                spin: (Math.random() - 0.5) * 0.3,
                color: palette[i % palette.length],
                life: 1,
                round: Math.random() < 0.3,
            });
        }
        if (!frame) frame = requestAnimationFrame(draw);
    }

    function draw() {
        ctx.clearRect(0, 0, innerWidth, innerHeight);
        pieces = pieces.filter((p) => p.life > 0 && p.y < innerHeight + 20);
        for (const p of pieces) {
            p.vy += 0.18;
            p.vx *= 0.99;
            p.x += p.vx;
            p.y += p.vy;
            p.rot += p.spin;
            p.life -= 0.006;
            ctx.save();
            ctx.globalAlpha = Math.min(1, p.life * 2);
            ctx.translate(p.x, p.y);
            ctx.rotate(p.rot);
            ctx.fillStyle = p.color;
            if (p.round) {
                ctx.beginPath();
                ctx.arc(0, 0, p.size / 2.5, 0, Math.PI * 2);
                ctx.fill();
            } else {
                ctx.fillRect(-p.size / 2, -p.size / 4, p.size, p.size / 2);
            }
            ctx.restore();
        }
        frame = pieces.length ? requestAnimationFrame(draw) : 0;
    }

    if (ctx) {
        resize();
        window.addEventListener("resize", resize);
    }

    if (today) {
        setTimeout(() => {
            burst(innerWidth * 0.2, innerHeight, 90, 1.1);
            burst(innerWidth * 0.8, innerHeight, 90, 1.1);
        }, 600);
    }

    // The extra: first tap reveals it, later taps copy it.
    function wireExtra(extra) {
        const value = extra.querySelector(".extra-value");
        const openLink = extra.parentElement.querySelector(".extra-open");
        extra.addEventListener("click", async () => {
            if (extra.classList.contains("is-hidden")) {
                extra.classList.remove("is-hidden");
                extra.setAttribute("aria-label", "Copy the surprise");
                if (openLink) openLink.hidden = false;
                const r = extra.getBoundingClientRect();
                burst(r.left + r.width / 2, r.top, 50, 1.4);
                return;
            }
            try {
                await navigator.clipboard.writeText(value.textContent.trim());
                extra.classList.add("is-copied");
                setTimeout(() => extra.classList.remove("is-copied"), 1600);
            } catch {
                // No clipboard (e.g. plain HTTP): select it for a manual copy.
                window.getSelection().selectAllChildren(value);
            }
        });
    }

    const extra = document.querySelector(".extra-code");
    if (extra) wireExtra(extra);

    // The chat friend. The server keeps nothing, so the page sends the whole
    // conversation with each message; it lasts until the page reloads.
    const chat = document.querySelector(".chat");
    let chatPIN = "";
    let unlockChat = () => {};
    if (chat) {
        const log = chat.querySelector(".chat-log");
        const form = chat.querySelector(".chat-form");
        const input = form.querySelector("textarea");
        const send = form.querySelector("button");
        const friend = chat.dataset.friend;
        const history = [...log.querySelectorAll(".chat-msg")].map((li) => ({ from: "friend", text: li.textContent.trim() }));

        const bubble = (cls, text) => {
            const li = document.createElement("li");
            li.className = `chat-msg ${cls}`;
            li.textContent = text;
            log.append(li);
            log.scrollTop = log.scrollHeight;
            return li;
        };

        unlockChat = (pin) => {
            if (!chat.classList.contains("is-locked")) return;
            chatPIN = pin;
            chat.classList.remove("is-locked");
            chat.querySelector(".chat-lock, .chat-pin")?.remove();
            log.hidden = false;
            form.hidden = false;
        };

        const grow = () => {
            input.style.height = "auto";
            input.style.height = `${input.scrollHeight}px`;
        };
        input.addEventListener("input", grow);
        input.addEventListener("keydown", (e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
                e.preventDefault();
                form.requestSubmit();
            }
        });

        form.addEventListener("submit", async (e) => {
            e.preventDefault();
            const text = input.value.trim();
            if (!text || send.disabled) return;
            send.disabled = true;
            input.value = "";
            grow();
            bubble("from-me", text);
            history.push({ from: "me", text });

            const reply = bubble("from-friend is-typing", "");
            reply.append(...[0, 1, 2].map(() => document.createElement("span")));
            let said = "";
            let failed = "";
            try {
                const res = await fetch("/api/chat", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ pin: chatPIN, messages: history }),
                });
                if (!res.ok) {
                    failed =
                        res.status === 429
                            ? `${friend} needs a tiny breather. Try again in a moment!`
                            : `${friend} couldn't get your message (${res.status}). Reload the page and try again.`;
                } else {
                    // One JSON object per line: {text}, then {done} or {error}.
                    const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
                    let buf = "";
                    let done = false;
                    while (!done) {
                        const chunk = await reader.read();
                        if (chunk.done) break;
                        buf += chunk.value;
                        let nl;
                        while ((nl = buf.indexOf("\n")) >= 0) {
                            const line = buf.slice(0, nl);
                            buf = buf.slice(nl + 1);
                            if (!line) continue;
                            const ev = JSON.parse(line);
                            if (ev.text) {
                                if (!said) reply.className = "chat-msg from-friend";
                                said += ev.text;
                                reply.textContent = said;
                                log.scrollTop = log.scrollHeight;
                            }
                            if (ev.error) failed = `${friend} stepped away for a moment. Try again in a bit!`;
                            if (ev.done || ev.error) done = true;
                        }
                    }
                    if (!done && !failed) failed = `${friend} got cut off. Try again?`;
                }
            } catch {
                failed = "Couldn't reach the server. Try again.";
            }

            if (said.trim()) {
                history.push({ from: "friend", text: said.trim() });
            } else {
                reply.remove();
            }
            if (failed) {
                bubble("is-note", failed);
                // Nothing came back: let the message be sent again.
                if (!said.trim()) {
                    history.pop();
                    input.value = text;
                    grow();
                }
            }
            send.disabled = false;
            input.focus();
        });
    }

    // A PIN form: check posts the PIN and returns the response; onOk runs
    // when the server takes it.
    function wirePin(pinForm, check, onOk) {
        const input = pinForm.querySelector("input");
        const button = pinForm.querySelector("button");
        const msg = pinForm.querySelector(".extra-pin-msg");
        pinForm.addEventListener("submit", async (e) => {
            e.preventDefault();
            const pin = input.value.trim();
            if (!pin) return;
            button.disabled = true;
            msg.textContent = "";
            try {
                const res = await check(pin);
                if (res.ok) {
                    await onOk(pin, res);
                    return;
                }
                if (res.status === 429) {
                    msg.textContent = "Too many tries. Wait a moment and try again.";
                } else if (res.status === 403) {
                    msg.textContent = "That's not it. Try again?";
                    input.value = "";
                } else {
                    msg.textContent = "Something went wrong. Reload the page and try again.";
                }
                pinForm.classList.remove("is-wrong");
                void pinForm.offsetWidth;
                pinForm.classList.add("is-wrong");
            } catch {
                msg.textContent = "Couldn't reach the server. Try again.";
            } finally {
                button.disabled = false;
                if (document.body.contains(input)) input.focus();
            }
        });
    }

    const post = (url, body) =>
        fetch(url, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
        });

    // PIN-guarded extra: the server only hands the secret over for the right
    // PIN, then it is shown already revealed. The same PIN unlocks the chat.
    const pinForm = document.querySelector(".extra-pin:not(.chat-pin)");
    if (pinForm) {
        wirePin(
            pinForm,
            (pin) => post("/api/reveal", { pin }),
            async (pin, res) => {
                const { secret, link } = await res.json();
                const code = document.createElement("button");
                code.type = "button";
                code.className = "extra-code";
                code.setAttribute("aria-label", "Copy the surprise");
                const value = document.createElement("span");
                value.className = "extra-value";
                value.textContent = secret;
                code.append(value);
                pinForm.replaceWith(code);
                if (link) {
                    const a = document.createElement("a");
                    a.className = "extra-open";
                    a.href = secret;
                    a.target = "_blank";
                    a.rel = "noopener noreferrer";
                    a.textContent = "Open";
                    code.after(a);
                }
                wireExtra(code);
                const r = code.getBoundingClientRect();
                burst(r.left + r.width / 2, r.top, 60, 1.4);
                unlockChat(pin);
            },
        );
    }

    // The chat's own PIN form, when there's no PIN-guarded extra to share.
    const chatPinForm = document.querySelector(".chat-pin");
    if (chatPinForm) {
        wirePin(
            chatPinForm,
            (pin) => post("/api/chat", { pin, messages: [] }),
            (pin) => {
                unlockChat(pin);
                chat.querySelector(".chat-form textarea").focus();
            },
        );
    }

    const glass = document.querySelector(".glass");
    if (glass) {
        glass.addEventListener("click", () => {
            glass.classList.remove("cheers");
            void glass.offsetWidth;
            glass.classList.add("cheers");
            const r = glass.getBoundingClientRect();
            burst(r.left + r.width / 2, r.top + r.height / 3, 40, 1.6);
        });
    }
})();
