import { check, fail } from 'k6'
import http from 'k6/http'
// import secrets from 'k6/secrets'

export default async () => {
	const my_secret = await secrets.get('secret_name');
	const result = http.get('https://grafana.com/', {
		headers: { 'X-My-Header': my_secret },
	});

	const pass = check(result, {
		'is status 200': (r) => r.status === 200,
	});

	if (!pass) {
		fail(`non 200 result ${result.status}`);
	}
}
