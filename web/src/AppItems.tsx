import { useEffect, useState } from "react";
import { canPin, pin, updater, type WidgetKind } from "./appBridge";

// То, что есть только внутри Android-приложения: «Обновить приложение» в
// меню и «на рабочий стол» у виджетов. APK и его версию отдаёт сервер
// (/app/, по сессии); о ходе обновления приложение рассказывает через
// window.pvAppUpdate(state, text): busy, latest, done, fail.

export function AppUpdate() {
  const [u] = useState(updater);
  const [label, setLabel] = useState("Обновить приложение");
  const [busy, setBusy] = useState(false);
  const [fresh, setFresh] = useState("");

  useEffect(() => {
    if (!u) return;
    fetch("/app/version.json", { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((v: { code: number; name: string } | null) => v && v.code > u.code && setFresh(v.name))
      .catch(() => {});
    let reset: ReturnType<typeof setTimeout>;
    (window as unknown as { pvAppUpdate: (s: string, t: string) => void }).pvAppUpdate = (state, text) => {
      setBusy(state === "busy");
      setLabel(text);
      clearTimeout(reset);
      if (state !== "busy") reset = setTimeout(() => setLabel("Обновить приложение"), 8000);
    };
    return () => clearTimeout(reset);
  }, [u]);

  if (!u) return null;
  return (
    <button className={fresh ? "side-link app-update fresh" : "side-link app-update"} onClick={() => !busy && u.run()} aria-busy={busy}>
      <span className="side-label">{label}</span>
      <span className="side-count">{fresh ? `есть ${fresh}` : u.name}</span>
    </button>
  );
}

export function PinWidget({ kind, label }: { kind: WidgetKind; label: string }) {
  const [show, setShow] = useState(() => canPin(kind));
  if (!show) return null;
  return (
    <button className="ghost pin" onClick={() => pin(kind) && setShow(false)}>
      {label} на рабочий стол
    </button>
  );
}
