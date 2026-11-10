// Иконки шапки — штрихом, в цвет текста. Эмодзи рисует каждая система
// по-своему, и рядом с надписями они выглядели чужими.

const base = {
  width: 18,
  height: 18,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.6,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
  "aria-hidden": true,
};

export const IconMenu = () => (
  <svg {...base}>
    <path d="M4 7h16M4 12h16M4 17h10" />
  </svg>
);

export const IconSearch = () => (
  <svg {...base}>
    <circle cx="11" cy="11" r="6.5" />
    <path d="m20 20-4.2-4.2" />
  </svg>
);

/** булавка — «Важное» */
export const IconPin = () => (
  <svg {...base}>
    <path d="M9 4h6l-1 5 3 3v2H7v-2l3-3-1-5zM12 14v6" />
  </svg>
);

export const IconKey = () => (
  <svg {...base}>
    <circle cx="8" cy="15" r="4" />
    <path d="m11 12 8-8M16 7l2 2M14 9l2 2" />
  </svg>
);

export const IconExit = () => (
  <svg {...base}>
    <path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4M10 16l-4-4 4-4M6 12h10" />
  </svg>
);

// нижнее меню телефона
export const IconHome = () => (
  <svg {...base}>
    <path d="M4 11.5 12 5l8 6.5" />
    <path d="M6.5 10v9h11v-9" />
  </svg>
);

export const IconInbox = () => (
  <svg {...base}>
    <path d="M4 13h4.5l1.5 2.5h4l1.5-2.5H20" />
    <path d="M6.5 5.5h11L20 13v5.5H4V13z" />
  </svg>
);

export const IconFood = () => (
  <svg {...base}>
    <path d="M4 12h16a8 8 0 0 1-16 0z" />
    <path d="M9 4.5c0 1.5 1 1.5 1 3M13 4.5c0 1.5 1 1.5 1 3" />
  </svg>
);

export const IconPlus = () => (
  <svg {...base}>
    <path d="M12 5v14M5 12h14" />
  </svg>
);

/** пузырь — «ИИ» */
export const IconChat = () => (
  <svg {...base}>
    <path d="M4.5 6.5A2.5 2.5 0 0 1 7 4h10a2.5 2.5 0 0 1 2.5 2.5v7A2.5 2.5 0 0 1 17 16H9.5L5 19.5V6.5z" />
  </svg>
);

/** вилка — «Подключения» */
export const IconPlug = () => (
  <svg {...base}>
    <path d="M9 7V3.5M15 7V3.5" />
    <path d="M7 7h10v3.5a5 5 0 0 1-10 0V7z" />
    <path d="M12 15.5V21" />
  </svg>
);
