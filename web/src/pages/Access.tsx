import { useState, type FormEvent } from "react";
import type { Access as AccessData } from "../api";
import { send } from "../api";
import { useToast } from "../toast";
import { useData } from "../useData";

// Доступ: кто и когда входил, смена ключа. Смена — формой POST
// /access; это устройство остаётся внутри, остальные попросят новый ключ.

export function Access() {
  const { data, reload } = useData<AccessData>("/api/access");
  const say = useToast();
  const [done, setDone] = useState("");

  async function change(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const f = new FormData(form);
    const res = await send("/access", { old: `${f.get("old")}`, new: `${f.get("new")}`, repeat: `${f.get("repeat")}` });
    if (!res.ok) {
      say(res.message || "Ключ не сменён");
      return;
    }
    form.reset();
    setDone(res.message || "Ключ сменён");
    await reload();
  }

  return (
    <div className="page access">
      <header className="list-head">
        <h1>Доступ</h1>
        <p>Кто входил в хранилище и смена ключа</p>
      </header>
      <section className="widget">
        <h2>Сменить ключ</h2>
        {data?.fromEnv ? (
          <p className="quiet">Ключ задан на сервере в окружении (PILLAR_PASS_HASH) и отсюда не меняется.</p>
        ) : (
          <form className="key-form" onSubmit={change} autoComplete="off">
            <label>Нынешний ключ<input type="password" name="old" required autoComplete="current-password" /></label>
            <label>Новый ключ<input type="password" name="new" required minLength={8} autoComplete="new-password" /></label>
            <label>Новый ещё раз<input type="password" name="repeat" required minLength={8} autoComplete="new-password" /></label>
            <button className="primary" type="submit">Сменить ключ</button>
            <p className="quiet">{done || "Перезапуск не нужен. Это устройство останется внутри, остальные — приложение и виджеты тоже — попросят новый ключ."}</p>
          </form>
        )}
      </section>
      <section className="widget">
        <h2>Последние входы</h2>
        {!data ? (
          <div className="loading" aria-busy />
        ) : data.logins.length === 0 ? (
          <p className="quiet">Записей пока нет.</p>
        ) : (
          <table className="logins">
            <thead>
              <tr><th>Когда</th><th>Что</th><th>Адрес</th><th className="wide-only">Устройство</th></tr>
            </thead>
            <tbody>
              {data.logins.map((l, i) => (
                <tr key={i} className={l.ok ? "ok" : "bad"}>
                  <td>{l.at}</td><td>{l.result}</td><td>{l.ip}</td><td className="wide-only">{l.device}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
