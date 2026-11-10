import { useEffect, useRef } from "react";

// Фон: открытое небо, мягкое солнце и дождь матрицы поверх. Слой fixed и
// под всем — прокрутка его не таскает. Цвета — токенами по фазе суток
// (styles.css, [data-sky]).

const GLYPHS = "アイウエオカキクケコサシスセソタチツテト0123456789";
const STEP = 22; // шаг колонн, px: редкий дождь — фон, а не стена
const FPS = 18;

export function Sky() {
  return (
    <div className="sky" aria-hidden>
      <div className="sun" />
      <Rain />
    </div>
  );
}

function Rain() {
  const ref = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    let drops: number[] = [];
    let w = 0;
    let h = 0;
    const size = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      w = canvas.clientWidth;
      h = canvas.clientHeight;
      canvas.width = w * dpr;
      canvas.height = h * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      drops = Array.from({ length: Math.ceil(w / STEP) }, () => Math.random() * -60);
    };
    size();

    // цвета дождя — из токенов фазы; сверяем раз в две секунды, не каждый кадр
    let rain = "";
    let head = "";
    let checked = -Infinity;
    const colors = (t: number) => {
      if (t - checked < 2000) return;
      checked = t;
      const css = getComputedStyle(document.documentElement);
      rain = css.getPropertyValue("--rain").trim() || "#2fb36b";
      head = css.getPropertyValue("--rain-head").trim() || "#d9ffe6";
    };

    let raf = 0;
    let last = 0;
    const frame = (t: number) => {
      raf = requestAnimationFrame(frame);
      if (t - last < 1000 / FPS) return;
      last = t;
      // гасим прошлый кадр, не трогая неба под холстом: след тает сам
      ctx.globalCompositeOperation = "destination-out";
      ctx.fillStyle = "rgba(0,0,0,0.14)";
      ctx.fillRect(0, 0, w, h);
      ctx.globalCompositeOperation = "source-over";
      colors(t);
      ctx.font = '14px "JetBrains Mono Variable", monospace';
      for (let i = 0; i < drops.length; i++) {
        const y = drops[i] * 16;
        ctx.fillStyle = Math.random() > 0.94 ? head : rain;
        ctx.fillText(GLYPHS[(Math.random() * GLYPHS.length) | 0], i * STEP + 4, y);
        drops[i] = y > h && Math.random() > 0.97 ? Math.random() * -20 : drops[i] + 1;
      }
    };
    raf = requestAnimationFrame(frame);

    // скрытая вкладка дождя не видит — и батарея его не оплачивает
    const vis = () => {
      cancelAnimationFrame(raf);
      if (!document.hidden) raf = requestAnimationFrame(frame);
    };
    document.addEventListener("visibilitychange", vis);
    window.addEventListener("resize", size);
    return () => {
      cancelAnimationFrame(raf);
      document.removeEventListener("visibilitychange", vis);
      window.removeEventListener("resize", size);
    };
  }, []);

  return <canvas ref={ref} className="rain" />;
}
