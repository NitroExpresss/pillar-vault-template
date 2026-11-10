// Часовой пояс хранилища — московский, а не пояс машины. VDS стоит в
// Европе (CET), и «сегодня», время еды, дневник и дела съезжали на два-три
// часа от того, что видит хозяин. Пояс задаётся здесь, а не в systemd:
// так он одинаков у сервера, агента, линтера и тестов. Другой — PILLAR_TZ.
package main

import (
	"log"
	"os"
	"time"
	_ "time/tzdata" // база поясов в бинаре: не зависим от tzdata на машине
)

const defaultTZ = "Europe/Moscow"

func init() {
	name := os.Getenv("PILLAR_TZ")
	if name == "" {
		name = defaultTZ
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("часовой пояс %q: %v — остаюсь на поясе машины", name, err)
		return
	}
	time.Local = loc
}
