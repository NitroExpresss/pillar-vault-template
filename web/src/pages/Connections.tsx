import { useState } from "react";
import { Link } from "react-router";
import { send } from "../api";
import { useAct } from "../useAct";
import { useData } from "../useData";
import { useToast } from "../toast";

// Шлюзы к моделям (OpenAI-совместимые). Ключ живёт только на сервере
// (server/.connections.json, 0600), сюда возвращается хвост. Проверка —
// живой запрос с сервера: без текста — ключ и список моделей, с текстом —
// короткий ответ модели. Удаление — вторым нажатием. Чат с моделями —
// вкладка «ИИ».

interface Preset {
  id: string;
  label: string;
  base_url: string;
  model: string;
  no_key?: boolean;
}

interface Conn {
  id: string;
  name: string;
  preset: string;
  base_url: string;
  model: string;
  key_tail: string;
  no_key?: boolean;
  created: string;
}

interface ConnData {
  connections: Conn[];
  presets: Preset[];
}

/** Правка подключения: название, адрес, модель; пустой ключ — без изменений. */
function EditConn({ c, reload, done }: { c: Conn; reload: () => Promise<void>; done: () => void }) {
  const act = useAct(reload);
  const [name, setName] = useState(c.name);
  const [baseURL, setBaseURL] = useState(c.base_url);
  const [model, setModel] = useState(c.model);
  const [key, setKey] = useState("");

  async function save() {
    const ok = await act(
      "/connections",
      { do: "update", id: c.id, name: name.trim(), base_url: baseURL.trim(), model: model.trim(), key: key.trim() },
      "Не сохранилось",
    );
    if (ok) done();
  }

  return (
    <div className="conn-form">
      <label>
        Название
        <input value={name} onChange={(e) => setName(e.target.value)} />
      </label>
      <label>
        Модель
        <input value={model} onChange={(e) => setModel(e.target.value)} />
      </label>
      <label className="wide">
        Адрес API
        <input value={baseURL} onChange={(e) => setBaseURL(e.target.value)} />
      </label>
      <label className="wide">
        Новый ключ (пусто — оставить прежний)
        <input type="password" value={key} autoComplete="off" onChange={(e) => setKey(e.target.value)} />
      </label>
      <div className="wide conn-acts">
        <button className="primary" disabled={!name.trim() || !baseURL.trim() || !model.trim()} onClick={() => void save()}>
          Сохранить
        </button>
        <button className="ghost" onClick={done}>
          Отмена
        </button>
      </div>
    </div>
  );
}

/** Одно подключение: сведения, проверка, правка, удаление. */
function ConnCard({ c, reload }: { c: Conn; reload: () => Promise<void> }) {
  const say = useToast();
  const act = useAct(reload);
  const [busy, setBusy] = useState(false);
  const [armed, setArmed] = useState(false);
  const [editing, setEditing] = useState(false);
  const [prompt, setPrompt] = useState("");
  const [reply, setReply] = useState("");
  const [failed, setFailed] = useState(false);

  /** Проверка: без текста — ключ и модели, с текстом — ответ модели. */
  async function test(withPrompt: boolean) {
    setBusy(true);
    setReply("");
    setFailed(false);
    const res = await send("/connections", { do: "test", id: c.id, prompt: withPrompt ? prompt : "" });
    setBusy(false);
    if (res.ok) {
      setReply(res.message || "…");
      if (withPrompt) setPrompt("");
    } else {
      setFailed(true);
      setReply(res.message || "Проверка не прошла");
      say(res.message || "Проверка не прошла");
    }
  }

  return (
    <section className="widget conn">
      <h2>
        {c.name}
        <Link className="ghost conn-chat" to={`/ai`}>
          в чат →
        </Link>
      </h2>
      {editing ? (
        <EditConn c={c} reload={reload} done={() => setEditing(false)} />
      ) : (
        <>
          <p className="conn-meta">
            {c.base_url} · {c.model}
            {c.no_key ? <> · без ключа</> : c.key_tail && <> · ключ …{c.key_tail}</>} · с {c.created}
          </p>
          {reply && <p className={failed ? "conn-reply fail" : "conn-reply"}>{reply}</p>}
          <div className="conn-acts">
            <button className="ghost" disabled={busy} onClick={() => void test(false)}>
              Проверить
            </button>
            <input
              type="text"
              value={prompt}
              placeholder="Спросить модель…"
              onChange={(e) => setPrompt(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && prompt.trim() && !busy) void test(true);
              }}
            />
            <button className="ghost" disabled={busy || !prompt.trim()} onClick={() => void test(true)}>
              Спросить
            </button>
            <button className="ghost" onClick={() => setEditing(true)}>
              Изменить
            </button>
            <button
              className={armed ? "ghost armed" : "ghost"}
              onClick={() => {
                if (!armed) return setArmed(true);
                setArmed(false);
                void act("/connections", { do: "remove", id: c.id }, "Не удалилось");
              }}
            >
              {armed ? "Точно удалить?" : "Удалить"}
            </button>
          </div>
        </>
      )}
    </section>
  );
}

/** Форма добавления: пресет подставляет адрес и модель. */
function AddConn({ presets, reload }: { presets: Preset[]; reload: () => Promise<void> }) {
  const act = useAct(reload);
  const [preset, setPreset] = useState(presets[0]?.id ?? "grok");
  const [name, setName] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [model, setModel] = useState("");
  const [key, setKey] = useState("");

  const p = presets.find((x) => x.id === preset);
  const custom = preset === "custom";
  const needKey = custom || !p?.no_key;

  async function add() {
    const ok = await act(
      "/connections",
      {
        do: "add",
        preset,
        name,
        base_url: custom ? baseURL : "",
        model: custom ? model : "",
        key,
      },
      "Не добавилось",
    );
    if (ok) {
      setName("");
      setBaseURL("");
      setModel("");
      setKey("");
    }
  }

  return (
    <section className="widget conn">
      <h2>Добавить</h2>
      <div className="conn-form">
        <label>
          Провайдер
          <select value={preset} onChange={(e) => setPreset(e.target.value)}>
            {presets.map((x) => (
              <option key={x.id} value={x.id}>
                {x.label}
              </option>
            ))}
            <option value="custom">Свой (OpenAI-совместимый)</option>
          </select>
        </label>
        <label>
          Название
          <input value={name} placeholder={custom ? "Мой шлюз" : p?.label} onChange={(e) => setName(e.target.value)} />
        </label>
        {custom ? (
          <>
            <label>
              Адрес API
              <input value={baseURL} placeholder="https://…/v1" onChange={(e) => setBaseURL(e.target.value)} />
            </label>
            <label>
              Модель
              <input value={model} placeholder="имя модели" onChange={(e) => setModel(e.target.value)} />
            </label>
          </>
        ) : (
          <p className="conn-meta wide">
            {p?.base_url} · модель {p?.model}
          </p>
        )}
        {needKey && (
          <label className="wide">
            Ключ API
            <input type="password" value={key} placeholder="xai-…" autoComplete="off" onChange={(e) => setKey(e.target.value)} />
          </label>
        )}
        <div className="wide">
          <button className="primary" onClick={() => void add()}>
            Добавить подключение
          </button>
        </div>
      </div>
    </section>
  );
}

export function Connections() {
  const { data, reload } = useData<ConnData>("/api/connections");

  if (!data) return <div className="page"><div className="loading" aria-busy /></div>;
  return (
    <div className="page connections">
      <header className="list-head">
        <h1>Шлюзы</h1>
        <p>
          Подключения к моделям: Grok, OpenAI-совместимые, локальный шлюз. Чат с ними — вкладка «ИИ». Ключ хранится
          на сервере, сюда возвращается только хвост.
        </p>
      </header>
      {data.connections.length === 0 ? (
        <section className="widget empty">
          <p>Пока ни одного шлюза — добавьте первый ниже.</p>
        </section>
      ) : (
        data.connections.map((c) => <ConnCard key={c.id} c={c} reload={reload} />)
      )}
      <AddConn presets={data.presets} reload={reload} />
    </div>
  );
}
