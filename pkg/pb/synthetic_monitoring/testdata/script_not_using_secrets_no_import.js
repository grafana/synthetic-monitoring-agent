import { check, fail } from 'k6'
import http from 'k6/http'

export default function() {
	const result = http.get('https://grafana.com/');

	const pass = check(result, {
		'is status 200': (r) => r.status === 200,
	});

	if (!pass) {
		fail(`non 200 result ${result.status}`);
	}
}
