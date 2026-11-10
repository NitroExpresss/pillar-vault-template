import { useState, type ReactNode } from "react";
import type { Board, Fast as FastData, Food, Nav, Shop, Vows } from "./api";
import { PinWidget } from "./AppItems";
import type { WidgetKind } from "./appBridge";
import { Body } from "./blocks/Body";
import { Calendar } from "./blocks/Calendar";
// «Сад» убран с главной 2026-10-22 (решение хозяина) — блок жив в blocks/Garden.tsx
// import { Garden } from "./blocks/Garden";
import { Fast } from "./blocks/Fast";
import { Goals } from "./blocks/Goals";
import { Deadlines, Diary, FoodLog, Gallery, Pins, Shopping, Tasks, Wheel } from "./blocks/Small";
import { Today } from "./blocks/Today";
import { splitDate } from "./lib";
import { greeting, phaseAt } from "./phase";
import { useAct } from "./useAct";
import { useData } from "./useData";

// Главная — командный центр: приветствие под небом и виджеты. Порядок
// виджетов задан здесь — переставлять их нечего и незачем.

export function Dashboard() {
  const vows = useData<Vows>("/vows.json");
  const nav = useData<Nav>("/api/nav");
  // выбранный месяц и день календаря — часть адреса данных: сменил день — пришёл его срез
  const [cal, setCal] = useState({ month: "", day: "" });
  const board = useData<Board>(`/api/board?cal=${cal.month}&day=${cal.day}`);
  const shop = useData<Shop>("/shop.json");
  const food = useData<Food>("/food.json");
  const fast = useData<FastData>("/fast.json");
  // запись еды обрывает голод — оба блока сверяются с сервером
  const actFood = useAct(async () => {
    await Promise.all([food.reload(), fast.reload()]);
  });
  const act = useAct(board.reload);
  const actShop = useAct(async () => {
    await Promise.all([shop.reload(), board.reload()]);
  });

  const day = vows.data ? splitDate(vows.data.date) : null;
  // для сводки «Сада» — вернуть вместе с ним
  // const total = vows.data?.vows.length ?? 0;
  // const held = vows.data?.vows.filter((v) => v.mark === "✓").length ?? 0;
  const b = board.data;

  return (
    <div className="page page-wide">
      <header className="hello">
        <p className="hello-greet">{greeting[phaseAt()]}</p>
        <h1 className="hello-date">{day ? `${day.date}, ${day.weekday}` : " "}</h1>
        {/* «Сад» убран — и его сводка под датой вместе с ним
        {total > 0 && (
          <p className="hello-tally">
            {held === total ? "Весь сад в цвету" : held === 0 ? "Сад ждёт отметок" : `В цвету ${held} из ${total}`}
          </p>
        )} */}
        {nav.data && nav.data.broken > 0 && (
          <a className="health-banner" href="/health">
            В хранилище {nav.data.broken} {nav.data.broken === 1 ? "ошибка" : nav.data.broken < 5 ? "ошибки" : "ошибок"} — посмотреть и починить
          </a>
        )}
        {nav.data && nav.data.inbox > 0 && (
          <a className="inbox-banner" href="/inbox">
            Во входящих {nav.data.inbox} — разобрать
          </a>
        )}
        {/* «Захват» — виджет без данных, на главной ему места нет: только кнопка на рабочий стол */}
        <PinWidget kind="capture" label="Захват" />
      </header>

      <div className="widgets">
        {/* «Сад» убран с главной 2026-10-22: полоса символов под ним выглядела мусором; вернуть — раскомментировать
        <Widget title="Сад" area="garden" pin={["vows", "Столпы"]} extra={<a href="/doc/pillars/inviolable">Устав</a>}>
          {vows.data ? <Garden data={vows.data} set={vows.set} reload={vows.reload} /> : <Loading />}
        </Widget> */}
        <Widget title="Сегодня" area="today" pin={["today", "Сегодня"]}>
          <Today />
        </Widget>
        {/* еда и голодание — один блок: голод считается от записанной еды */}
        <Widget title="Еда и голодание" area="food" pin={["fast", "Голодание"]} extra={<a href="/doc/notes/food">Весь журнал</a>}>
          <div className="eat">
            {fast.data ? <Fast fast={fast.data} act={actFood} /> : <Loading />}
            <div className="eat-log">
              <h3>Сегодня</h3>
              {food.data ? <FoodLog food={food.data} act={actFood} /> : <Loading />}
            </div>
          </div>
          <PinWidget kind="food" label="Еда" />
        </Widget>
        {/* «Важное» — третьим, сразу после еды (решение хозяина 2026-10-27) */}
        <Widget title="Важное" area="pins" pin={["pins", "Важное"]}>
          {b ? <Pins pins={b.pins ?? []} act={act} /> : <Loading />}
        </Widget>
        <Widget title="Что горит" area="burn">
          {b ? <Deadlines zones={b.deadlines} act={act} /> : <Loading />}
        </Widget>
        {/* «Покупки» — в пару к «Что горит»: два полуширинных в ряд */}
        <Widget title="Покупки" area="shop" pin={["shop", "Покупки"]}>
          {shop.data ? <Shopping shop={shop.data} act={actShop} /> : <Loading />}
        </Widget>
        <Widget title="Цели" area="goals" pin={["goals", "Цели"]} extra={<a href="/doc/pillars/life-goals">Все цели года</a>}>
          {b ? <Goals goals={b.goals ?? []} today={b.today} act={act} /> : <Loading />}
        </Widget>
        <Widget title="Календарь" area="cal" pin={["calendar", "Календарь"]}>
          {b ? <Calendar cal={b.calendar} act={act} go={(month, day = "") => setCal({ month, day })} /> : <Loading />}
        </Widget>
        <Widget title="Дневник" area="diary" extra={<a href="/doc/notes/diary">Весь дневник</a>}>
          {b ? <Diary entries={b.diary} act={act} /> : <Loading />}
        </Widget>
        {b?.body && (
          <Widget title="Тело" area="body" pin={["body", "График тела"]} extra={<a href={b.body.url}>Замеры</a>}>
            <Body body={b.body} today={b.today} act={act} />
          </Widget>
        )}
        {b && b.wheel.length > 0 && (
          <Widget title="Колесо баланса" area="wheel" extra={<a href="/doc/pillars/life-goals">Цели</a>}>
            <Wheel spheres={b.wheel} />
          </Widget>
        )}
        {b?.tasks && (
          <Widget title="Учёт дел" area="ledger">
            <Tasks tasks={b.tasks} act={act} />
          </Widget>
        )}
        {b?.gallery && (
          <Widget title={b.gallery.title || "Витрина ориентиров"} area="gallery">
            <Gallery gallery={b.gallery} reload={board.reload} />
          </Widget>
        )}
      </div>
    </div>
  );
}

const Loading = () => <div className="loading" aria-busy />;

function Widget({ title, area, extra, pin, children }: {
  title: string;
  area: string;
  extra?: ReactNode;
  pin?: [WidgetKind, string]; // виджет Android, который можно поставить на рабочий стол
  children: ReactNode;
}) {
  const id = `w-${area}`;
  return (
    <section className={`widget w-${area}`} aria-labelledby={id}>
      <header className="widget-head">
        <h2 id={id}>{title}</h2>
        {extra}
      </header>
      {children}
      {pin && <PinWidget kind={pin[0]} label={pin[1]} />}
    </section>
  );
}
