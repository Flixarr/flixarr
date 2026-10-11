// Minimal 5-field cron: parse, describe in plain English, compute next run times (mirrors internal/jobs/cron.go).
// Fields: minute hour day-of-month month day-of-week (0–6, Sun = 0; 7 also = Sun).
const RANGES: [number, number][] = [[0, 59], [0, 23], [1, 31], [1, 12], [0, 7]];
const DAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

function field(src: string, [lo, hi]: [number, number]) {
	const out = new Set<number>();
	for (const part of src.split(',')) {
		const m = part.match(/^(\*|\d+(?:-\d+)?)(?:\/(\d+))?$/);
		if (!m) throw new Error(`“${part}” isn't valid`);
		let [a, b] = m[1] === '*' ? [lo, hi] : m[1].split('-').map(Number);
		if (b === undefined) b = m[2] ? hi : a;
		const step = m[2] ? +m[2] : 1;
		if (a < lo || b > hi || a > b || step < 1) throw new Error(`“${part}” is out of range (${lo}–${hi})`);
		for (let v = a; v <= b; v += step) out.add(v);
	}
	return out;
}

export function parse(expr: string) {
	const f = expr.trim().split(/\s+/);
	if (f.length !== 5) throw new Error('A schedule needs 5 parts: minute hour day month weekday');
	const [min, hr, dom, mon, dow] = f.map((x, i) => field(x, RANGES[i]));
	if (dow.has(7)) dow.add(0);
	return { min, hr, dom, mon, dow, domAny: f[2] === '*', dowAny: f[4] === '*', raw: f };
}

export function next(expr: string, count = 5, from = new Date()) {
	const c = parse(expr);
	const out: Date[] = [];
	const d = new Date(from);
	d.setSeconds(0, 0);
	d.setMinutes(d.getMinutes() + 1);
	for (let day = 0; day < 400 && out.length < count; day++) {
		const base = new Date(d.getFullYear(), d.getMonth(), d.getDate() + day);
		if (!c.mon.has(base.getMonth() + 1)) continue;
		const domOk = c.dom.has(base.getDate()), dowOk = c.dow.has(base.getDay());
		const dayOk = c.domAny && c.dowAny ? true : c.domAny ? dowOk : c.dowAny ? domOk : domOk || dowOk;
		if (!dayOk) continue;
		for (const h of [...c.hr].sort((a, b) => a - b))
			for (const mi of [...c.min].sort((a, b) => a - b)) {
				const t = new Date(base.getFullYear(), base.getMonth(), base.getDate(), h, mi);
				if (t >= d && out.length < count) out.push(t);
			}
	}
	return out;
}

const time = (h: number, m: number) => new Date(2000, 0, 1, h, m).toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' });

export function describe(expr: string) {
	const c = parse(expr);
	const [m, h, dom, mon, dow] = c.raw;
	const every = m.match(/^\*\/(\d+)$/), everyH = h.match(/^\*\/(\d+)$/);
	const one = (s: string) => /^\d+$/.test(s);
	if (m === '*' && h === '*' && dom === '*' && mon === '*' && dow === '*') return 'Every minute';
	if (every && h === '*' && dom === '*' && mon === '*' && dow === '*') return `Every ${every[1]} minutes`;
	if (one(m) && h === '*' && dom === '*' && mon === '*' && dow === '*') return `Every hour at :${m.padStart(2, '0')}`;
	if (one(m) && everyH && dom === '*' && mon === '*' && dow === '*') return `Every ${everyH[1]} hours at :${m.padStart(2, '0')}`;
	if (one(m) && one(h)) {
		const at = time(+h, +m);
		if (dom === '*' && mon === '*' && dow === '*') return `Every day at ${at}`;
		if (dom === '*' && mon === '*' && dow === '1-5') return `Weekdays at ${at}`;
		if (dom === '*' && mon === '*' && /^[\d,]+$/.test(dow)) return `Every ${dow.split(',').map((d) => DAYS[+d % 7]).join(', ')} at ${at}`;
		if (one(dom) && mon === '*' && dow === '*') return `Monthly on day ${dom} at ${at}`;
		if (one(dom) && one(mon) && dow === '*') return `Every year on ${MONTHS[+mon - 1]} ${dom} at ${at}`;
	}
	return 'Custom schedule';
}

export const fmtRun = (t: Date | string) =>
	new Date(t).toLocaleString('en-US', { weekday: 'short', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
