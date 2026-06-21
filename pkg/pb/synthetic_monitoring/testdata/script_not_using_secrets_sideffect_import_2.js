import { check, fail } from 'k6'
import http from 'k6/http'

// k6/secrets does not have documented side-effects, the following code shouldn't do anything. For
// that reason, this script is not considered as using secrets.
import "k6/secrets"

export default function() {
	const result = http.get('https://grafana.com/');

	const pass = check(result, {
		'is status 200': (r) => r.status === 200,
	});

	if (!pass) {
		fail(`non 200 result ${result.status}`);
	}
}
