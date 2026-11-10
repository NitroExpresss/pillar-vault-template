// Форма входа: дождь матрицы на небе и бегущие символы вместо ключа.
// Всё остальное — в приложении (web/), которое гостю не отдаётся.
(function () {
  "use strict";
  var GLYPHS = "アイウエオカキクケコサシスセソタチツテト0123456789";
  var still = matchMedia("(prefers-reduced-motion: reduce)").matches;

  // ключ: число символов видно, сами символы — нет
  var input = document.querySelector(".gate-input");
  var slots = document.querySelector(".gate-slots");
  if (input && slots) {
    input.addEventListener("input", function () {
      var n = Array.from(input.value).length;
      var s = "";
      for (var i = 0; i < Math.min(n, 18); i++) s += GLYPHS[(Math.random() * GLYPHS.length) | 0];
      slots.textContent = n > 18 ? s + "…" : s;
    });
  }

  var canvas = document.querySelector(".rain");
  if (!canvas || still) return;
  var ctx = canvas.getContext("2d");
  var STEP = 22, FPS = 18, drops = [], w = 0, h = 0, last = 0;
  var color = getComputedStyle(document.documentElement).getPropertyValue("--rain").trim() || "#2fb36b";
  function size() {
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    w = canvas.clientWidth; h = canvas.clientHeight;
    canvas.width = w * dpr; canvas.height = h * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    drops = Array.from({ length: Math.ceil(w / STEP) }, function () { return Math.random() * -60; });
  }
  function frame(t) {
    requestAnimationFrame(frame);
    if (document.hidden || t - last < 1000 / FPS) return;
    last = t;
    ctx.globalCompositeOperation = "destination-out"; // след тает, небо под холстом не трогаем
    ctx.fillStyle = "rgba(0,0,0,0.14)";
    ctx.fillRect(0, 0, w, h);
    ctx.globalCompositeOperation = "source-over";
    ctx.font = '14px "JetBrains Mono", monospace';
    ctx.fillStyle = color;
    for (var i = 0; i < drops.length; i++) {
      var y = drops[i] * 16;
      ctx.fillText(GLYPHS[(Math.random() * GLYPHS.length) | 0], i * STEP + 4, y);
      drops[i] = y > h && Math.random() > 0.97 ? Math.random() * -20 : drops[i] + 1;
    }
  }
  size();
  window.addEventListener("resize", size);
  requestAnimationFrame(frame);
})();
