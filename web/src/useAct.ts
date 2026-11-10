import { useCallback } from "react";
import { send } from "./api";
import { useToast } from "./toast";

export type Act = (path: string, fields: Record<string, string>, fallback?: string) => Promise<boolean>;

/**
 * Запись в хранилище и сверка с сервером: отказ — уведомлением с причиной,
 * после любого исхода — reload, чтобы экран показывал то, что в карточках.
 */
export function useAct(reload: () => Promise<void>): Act {
  const say = useToast();
  return useCallback(
    async (path: string, fields: Record<string, string>, fallback = "Не получилось") => {
      navigator.vibrate?.(12);
      const res = await send(path, fields);
      if (!res.ok) say(res.message || fallback);
      await reload();
      return res.ok;
    },
    [reload, say],
  );
}
