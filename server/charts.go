// Графики динамики тела — SVG, собранный на сервере.
//
// Раньше их рисовал app.js. Правила остались прежние: линии 2px, маркеры
// заметные, подпись ряда стоит прямо у последней точки (различаем не только
// цветом), сетка приглушена. Перекрестье с тултипом заменено на <title>
// у точки: то же значение по наведению, но без единой строки JS.
package main

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

type chartSeries struct {
	Name   string
	Values []*float64
	Color  string
}

// charts — вес и объёмы. Разные единицы, поэтому два графика: общая шкала
// исказила бы обе величины.
func charts(m *Metrics) []template.HTML {
	if m == nil || len(m.Rows) == 0 {
		return nil
	}
	labels := make([]string, 0, len(m.Rows))
	for _, r := range m.Rows {
		labels = append(labels, r.Date)
	}
	col := func(i int) string {
		if i+1 < len(m.Columns) {
			return m.Columns[i+1]
		}
		return fmt.Sprintf("ряд %d", i+1)
	}
	vals := func(i int) []*float64 {
		out := make([]*float64, 0, len(m.Rows))
		for _, r := range m.Rows {
			if i < len(r.Values) {
				out = append(out, r.Values[i])
			} else {
				out = append(out, nil)
			}
		}
		return out
	}
	nCols := 0
	for _, r := range m.Rows {
		if len(r.Values) > nCols {
			nCols = len(r.Values)
		}
	}
	if nCols == 0 {
		return nil
	}

	out := []template.HTML{lineChart(shortName(col(0)), unitOf(col(0)), labels,
		[]chartSeries{{Name: col(0), Values: vals(0), Color: "var(--viz-accent)"}})}

	volume := []chartSeries{}
	for i := 1; i < nCols; i++ {
		volume = append(volume, chartSeries{Name: col(i), Values: vals(i), Color: fmt.Sprintf("var(--viz-%d)", i)})
	}
	if len(volume) > 0 {
		out = append(out, lineChart("Объёмы", unitOf(col(1)), labels, volume))
	}
	return out
}

func unitOf(name string) string {
	if i := strings.LastIndex(name, ","); i >= 0 {
		return strings.TrimSpace(name[i+1:])
	}
	return ""
}

func shortName(name string) string {
	if i := strings.LastIndex(name, ","); i >= 0 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

func lineChart(title, unit string, labels []string, series []chartSeries) template.HTML {
	const (
		W, H       = 640.0, 232.0
		PL, PR     = 48.0, 104.0
		PT, PB     = 18.0, 30.0
		gridSlices = 3
	)
	min, max := math.Inf(1), math.Inf(-1)
	for _, s := range series {
		for _, v := range s.Values {
			if v == nil {
				continue
			}
			min = math.Min(min, *v)
			max = math.Max(max, *v)
		}
	}
	if math.IsInf(min, 1) {
		return ""
	}
	pad := (max - min) * 0.15
	if pad == 0 {
		pad = 1
	}
	min, max = min-pad, max+pad

	X := func(i int) float64 {
		if len(labels) < 2 {
			return PL
		}
		return PL + float64(i)*(W-PL-PR)/float64(len(labels)-1)
	}
	Y := func(v float64) float64 { return PT + (max-v)*(H-PT-PB)/(max-min) }

	var b strings.Builder
	esc := template.HTMLEscapeString
	fmt.Fprintf(&b, `<figure class="chart"><figcaption class="chart-cap">%s`, esc(title))
	if unit != "" {
		fmt.Fprintf(&b, ` <span>%s</span>`, esc(unit))
	}
	b.WriteString(`</figcaption>`)
	fmt.Fprintf(&b, `<svg viewBox="0 0 %g %g" class="chart-svg" role="img" aria-label="%s: с %s по %s">`,
		W, H, esc(title), esc(labels[0]), esc(labels[len(labels)-1]))

	for k := 0; k <= gridSlices; k++ {
		v := min + (max-min)*float64(k)/gridSlices
		y := Y(v)
		fmt.Fprintf(&b, `<line x1="%g" x2="%g" y1="%.1f" y2="%.1f" class="grid"/>`, PL, W-PR, y, y)
		digits := 1
		if math.Abs(v) >= 100 {
			digits = 0
		}
		fmt.Fprintf(&b, `<text x="%g" y="%.1f" class="axis" text-anchor="end">%.*f</text>`, PL-8, y+4, digits, v)
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%g" class="axis" text-anchor="start">%s</text>`, X(0), H-8, esc(shortDate(labels[0])))
	fmt.Fprintf(&b, `<text x="%.1f" y="%g" class="axis" text-anchor="end">%s</text>`,
		X(len(labels)-1), H-8, esc(shortDate(labels[len(labels)-1])))

	for _, s := range series {
		pts := [][2]float64{}
		lastV := 0.0
		for i, v := range s.Values {
			if v == nil {
				continue
			}
			pts = append(pts, [2]float64{X(i), Y(*v)})
			lastV = *v
		}
		if len(pts) == 0 {
			continue
		}
		poly := make([]string, 0, len(pts))
		for _, p := range pts {
			poly = append(poly, fmt.Sprintf("%.1f,%.1f", p[0], p[1]))
		}
		fmt.Fprintf(&b, `<polyline points="%s" class="line" stroke="%s"/>`, strings.Join(poly, " "), esc(s.Color))
		j := 0
		for i, v := range s.Values {
			if v == nil {
				continue
			}
			p := pts[j]
			j++
			// <title> вместо перекрестья на JS: значение по наведению, ноль скриптов
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="4.5" class="dot" fill="%s"><title>%s — %s%s</title></circle>`,
				p[0], p[1], esc(s.Color), esc(labels[i]), esc(num(*v)), unitSuffix(unit))
		}
		last := pts[len(pts)-1]
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="3.5" fill="%s"/>`, last[0]+12, last[1], esc(s.Color))
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="slabel">%s %s</text>`,
			last[0]+20, last[1]+4, esc(shortName(s.Name)), esc(num(lastV)))
	}
	b.WriteString(`</svg>`)

	if len(series) > 1 { // легенда обязательна от двух рядов
		b.WriteString(`<div class="chart-legend">`)
		for _, s := range series {
			fmt.Fprintf(&b, `<span class="cl"><span class="sw" style="background:%s"></span>%s</span>`,
				esc(s.Color), esc(shortName(s.Name)))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</figure>`)
	return template.HTML(b.String())
}

func shortDate(d string) string {
	if len(d) > 5 {
		return d[5:]
	}
	return d
}

func unitSuffix(unit string) string {
	if unit == "" {
		return ""
	}
	return " " + template.HTMLEscapeString(unit)
}
