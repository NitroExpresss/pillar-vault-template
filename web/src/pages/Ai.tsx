import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { postJSON } from "../api";
import { useData } from "../useData";
import { useToast } from "../toast";
import "./ai-setup.css";

// Чат с моделью через подключённые шлюзы. Переключение шлюза — на лету,
// история — в браузере (сервер ничего не хранит), карточка-контекст —
// из «Обсудить с ИИ» на странице карточки. Проверка шлюза — кнопкой
// или первым же ответом: точка на чипе зеленеет или краснеет.
// Шлюзов нет — вкладка сама становится мастером первого подключения:
// выбор провайдера, ключ, проверка живым запросом (сервер: do=add&verify=1).

interface Conn {
  id: string;
  name: string;
  model: string;
}

interface Preset {
  id: string;
  label: string;
  base_url: string;
  model: string;
  no_key?: boolean;
}

interface ConnData {
  connections: Conn[];
  presets: Preset[];
}

interface Msg {
  role: "user" | "assistant";
  text: string;
}

interface ChatReply {
  ok: boolean;
  message?: string;
  model?: string;
  conn?: string;
}

const LS_CHAT = "pv.ai.chat";
const LS_CONN = "pv.ai.conn";

const loadMsgs = (): Msg[] => {
  try {
    const m = JSON.parse(localStorage.getItem(LS_CHAT) ?? "[]") as Msg[];
    return Array.isArray(m) ? m.filter((x) => x && (x.role === "user" || x.role === "assistant") && typeof x.text === "string") : [];
  } catch {
    return [];
  }
};

// Первое подключение: провайдер → ключ → «Проверить и подключить».
// Сервер до записи зовёт модель живым запросом: битый ключ не сохраняется,
// причина возвращается текстом. Успех — галка и переход в чат (done → reload).
function AiSetup({ presets, done }: { presets: Preset[]; done: () => Promise<void> }) {
  const [preset, setPreset] = useState(() => presets.find((p) => p.id === "groq")?.id ?? presets[0]?.id ?? "");
  const [key, setKey] = useState("");
  const [show, setShow] = useState(false);
  const [phase, setPhase] = useState<"idle" | "checking" | "ok" | "fail">("idle");
  const [err, setErr] = useState("");

  const p = presets.find((x) => x.id === preset);
  const needKey = !p?.no_key;
  const checking = phase === "checking";

  async function connect() {
    if (checking || phase === "ok") return;
    setPhase("checking");
    setErr("");
    const res = await postJSON<{ ok: boolean; message?: string }>("/connections", {
      do: "add",
      preset,
      key: key.trim(),
      verify: "1",
    });
    if (res.ok) {
      setPhase("ok");
      window.setTimeout(() => void done(), 1100);
    } else {
      setErr(res.message || "Проверка не прошла");
      setPhase("fail");
    }
  }

  if (phase === "ok") {
    return (
      <div className="page">
        <section className="widget ai-setup">
          <div className="ai-setup-done">
            <svg className="ai-setup-mark" viewBox="0 0 52 52" aria-hidden>
              <circle cx="26" cy="26" r="24" />
              <path d="M15 27l7 7 15-16" />
            </svg>
            <p>Шлюз подключён — открываю чат…</p>
          </div>
        </section>
      </div>
    );
  }

  return (
    <div className="page">
      <section className="widget ai-setup">
        <h1>Подключите ИИ</h1>
        <p>
          Чату нужен ключ любого OpenAI-совместимого провайдера. Проще всего — Groq: ключ бесплатный и делается за
          минуту на console.groq.com. Ключ хранится только на вашем сервере, наружу не уходит.
        </p>
        <div className="ai-provs">
          {presets.map((x) => (
            <button
              key={x.id}
              className={x.id === preset ? "ai-prov on" : "ai-prov"}
              disabled={checking}
              onClick={() => setPreset(x.id)}
            >
              <b>
                {x.label}
                {x.id === "groq" ? " · рекомендуем" : ""}
              </b>
              <small>{x.no_key ? "без ключа" : x.model}</small>
            </button>
          ))}
        </div>
        {needKey && (
          <div className={phase === "fail" ? "ai-key shake" : "ai-key"} key={err || "key"}>
            <input
              type={show ? "text" : "password"}
              value={key}
              placeholder={preset === "groq" ? "gsk_…" : "ключ API"}
              autoComplete="off"
              disabled={checking}
              onChange={(e) => setKey(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && key.trim()) void connect();
              }}
            />
            <button className="ghost" disabled={checking} onClick={() => setShow(!show)}>
              {show ? "Скрыть" : "Показать"}
            </button>
          </div>
        )}
        {err && <p className="ai-setup-err">{err}</p>}
        <div>
          <button className="primary" disabled={checking || (needKey && !key.trim())} onClick={() => void connect()}>
            {checking && <span className="ai-spin" />}
            {checking ? "Проверяю ключ…" : "Проверить и подключить"}
          </button>
        </div>
        <p className="ai-setup-note">
          Свой адрес API или несколько шлюзов — на странице <Link to="/connections">«Шлюзы»</Link>.
        </p>
      </section>
    </div>
  );
}

export function Ai() {
  const { data, reload } = useData<ConnData>("/api/connections");
  const say = useToast();
  const [params] = useSearchParams();
  const [msgs, setMsgs] = useState<Msg[]>(loadMsgs);
  const [connId, setConnId] = useState(() => localStorage.getItem(LS_CONN) ?? "");
  const [card, setCard] = useState(() => params.get("card") ?? "");
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(false);
  const [status, setStatus] = useState<Record<string, "ok" | "fail">>({});
  const tailRef = useRef<HTMLDivElement>(null);

  const conns = data?.connections ?? [];
  const active = conns.find((c) => c.id === connId) ?? conns[0];

  useEffect(() => {
    localStorage.setItem(LS_CHAT, JSON.stringify(msgs.slice(-100)));
    tailRef.current?.scrollIntoView({ block: "end" });
  }, [msgs]);

  // При открытии вкладки — тихая проверка всех шлюзов: точки на чипах
  // сразу показывают, кто жив, без нажатия «Проверить».
  const autoChecked = useRef(false);
  useEffect(() => {
    if (autoChecked.current || !data) return;
    autoChecked.current = true;
    for (const c of data.connections ?? []) {
      void postJSON<{ ok: boolean }>("/connections", { do: "test", id: c.id, prompt: "" }).then((res) => {
        setStatus((s) => ({ ...s, [c.id]: res.ok ? "ok" : "fail" }));
      });
    }
  }, [data]);

  function choose(id: string) {
    setConnId(id);
    localStorage.setItem(LS_CONN, id);
  }

  async function sendMsg() {
    const text = input.trim();
    if (!text || busy || !active) return;
    const next = [...msgs, { role: "user" as const, text }];
    setMsgs(next);
    setInput("");
    setBusy(true);
    const res = await postJSON<ChatReply>("/api/ai/chat", {
      conn: active.id,
      messages: JSON.stringify(next.map((m) => ({ role: m.role, content: m.text }))),
      card,
    });
    setBusy(false);
    if (res.ok) {
      setMsgs([...next, { role: "assistant", text: res.message || "…" }]);
      setStatus((s) => ({ ...s, [active.id]: "ok" }));
    } else {
      say(res.message || "Модель не ответила");
      setStatus((s) => ({ ...s, [active.id]: "fail" }));
    }
  }

  async function check() {
    if (!active || checking) return;
    setChecking(true);
    const res = await postJSON<{ ok: boolean; message?: string }>("/connections", { do: "test", id: active.id, prompt: "" });
    setChecking(false);
    setStatus((s) => ({ ...s, [active.id]: res.ok ? "ok" : "fail" }));
    say(res.message || (res.ok ? "Шлюз отвечает" : "Проверка не прошла"));
  }

  function clear() {
    setMsgs([]);
    localStorage.removeItem(LS_CHAT);
  }

  // Шлюзов нет — мастер первого подключения вместо пустого чата.
  if (data && conns.length === 0) {
    return <AiSetup presets={data.presets ?? []} done={reload} />;
  }

  return (
    <div className="page ai">
      <div className="ai-bar">
        {conns.map((c) => (
          <button
            key={c.id}
            className={active?.id === c.id ? "ai-conn on" : "ai-conn"}
            onClick={() => choose(c.id)}
            title={c.model}
          >
            <span className={status[c.id] ? `ai-dot ${status[c.id]}` : "ai-dot"} />
            {c.name}
          </button>
        ))}
        <button className="ghost ai-mini" disabled={checking || !active} onClick={() => void check()}>
          {checking ? "Проверяю…" : "Проверить"}
        </button>
        <Link className="ghost ai-mini" to="/connections">
          Шлюзы
        </Link>
      </div>
      {card && (
        <p className="ai-context">
          Контекст: <span className="chip">{card}</span>
          <button className="ghost ai-mini" onClick={() => setCard("")}>
            убрать
          </button>
        </p>
      )}
      <div className="ai-msgs">
        {msgs.length === 0 && (
          <p className="ai-hint">
            Спроси что-нибудь: модель отвечает через шлюз «{active?.name}»{card ? ", карточка у неё в контексте" : ""}.
          </p>
        )}
        {msgs.map((m, i) => (
          <div key={i} className={m.role === "user" ? "ai-msg user" : "ai-msg bot"}>
            {m.text}
          </div>
        ))}
        {busy && <div className="ai-msg bot ai-thinking">думаю…</div>}
        <div ref={tailRef} />
      </div>
      <div className="ai-input">
        <input
          value={input}
          placeholder={busy ? "Модель отвечает…" : "Сообщение…"}
          disabled={busy}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void sendMsg();
          }}
        />
        <button className="primary" disabled={busy || !input.trim() || !active} onClick={() => void sendMsg()}>
          →
        </button>
        {msgs.length > 0 && (
          <button className="ghost" onClick={clear}>
            Очистить
          </button>
        )}
      </div>
    </div>
  );
}
