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

    // When the target passes, ask the server what state we are in now and
    // reload only once it agrees, so clock drift never causes a reload loop.
    async function settle() {
        try {
            const res = await fetch("/api/countdown", { cache: "no-store" });
            if (res.ok) {
                const m = await res.json();
                if (m.today !== today || Date.parse(m.start) !== start) {
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
        if (left <= 0) {
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

    const seal = document.querySelector(".seal");
    if (seal) {
        const meta = seal.parentElement.querySelector(".iou-meta");
        const original = meta.textContent;
        let restore = 0;
        seal.addEventListener("click", () => {
            seal.classList.remove("nope");
            void seal.offsetWidth;
            seal.classList.add("nope");
            const days = Number(seal.dataset.days);
            meta.textContent = days === 1 ? "Not yet! One more sleep." : `Not yet! ${days} sleeps to go.`;
            clearTimeout(restore);
            restore = setTimeout(() => {
                meta.textContent = original;
            }, 2500);
        });
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
