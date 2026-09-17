import http from 'k6/http';
import { check, fail } from 'k6';
import { test } from 'k6/execution';
// TODO(mem): conditionally import these modules
// - import encoding if base64 decoding is required
// - import jsonpath if there are json assertions
import encoding from 'k6/encoding';
import jsonpath from 'https://jslib.k6.io/jsonpath/1.0.2/index.js';
import { URL } from 'https://jslib.k6.io/url/1.0.0/index.js';

export const options = {
	scenarios: {
		default: {
			executor: 'shared-iterations',
			tags: {
				// TODO(mem): build tags out of options for the check?
				environment: 'production',
			},
			// exec: 'runner',
			maxDuration: '10s', // TODO(mem): this would be the timeout for the check
			gracefulStop: '1s',
		},
	},

	dns: {
		ttl: '2m', // TODO(mem): this doesn't need to be much higher than the maxDuration
		select: 'first',
		// TODO(mem): we can build this maps to IP option in checks, more or less
		policy: 'preferIPv4', // preferIPv6, onlyIPv4, onlyIPv6, any
	},

	// TODO(mem): we can build this out of check options
	insecureSkipTLSVerify: false,
	tlsVersion: {
		// TODO(mem): we can build this out of check options
		min: 'tls1.2',
		max: 'tls1.3',
	},
	// TODO(mem): we can build this out of agent version
	userAgent: 'synthetic-monitoring-agent/v0.14.3 (linux amd64; g64b8bab; +https://github.com/grafana/synthetic-monitoring-agent)',

	maxRedirects: 10,

	// k6 options
	vus: 1,
	// linger: false,
	summaryTimeUnit: 's',
	discardResponseBodies: false, // enable only if there are checks?
};

function assertHeader(headers, name, matcher) {
	const lcName = name.toLowerCase();
	const values = Object.entries(headers).
		filter(h => h[0].toLowerCase() === lcName).
		map(h => h[1]);

	if (values.find(v => matcher(v)) !== undefined) {
		return true;
	} else if (values.length === 0) {
		console.warn(`'${name}' not present in response`);
	} else {
		values.forEach(v => console.warn(`'${name}' has the value '${v}'`));
	}

	return false;
}

export default function() {
	let response;
	let body;
	let url;
	let currentCheck;
	let match;
	const logResponse = false
	const vars = {};

 
	console.log("Starting request to http://example.org/login...");
	try {
		url = new URL('http://example.org/login');
	} catch(e) {
		console.error("Invalid URL: http://example.org/login");
		fail()
	}
	

	body = encoding.b64decode("eyJ1c2VyIjoiY2hlY2stdXNlciJ9", 'rawstd', "s");
	
	response = http.request('POST', url.toString(), body, {
		// TODO(mem): build params out of options for the check
		tags: {
		  name: '0', // TODO(mem): give the user some control over this?
		  __raw_url__: 'http://example.org/login',
		},
		redirects: 0,
		headers: {'Content-Type':"application/json"}
	});
	console.log("Response received from http://example.org/login, status", response.status);
	if(logResponse) {
		const body = response.body || ''
		console.log("Response body received from http://example.org/login:", body.slice(0, 1000));
	}
	if(response.error) {
		console.error("Request error:" + url.toString() + ": " + response.error)
	}
	currentCheck = check(response, { "status code equals \"200\"": response => response.status.toString() === "200" }, {"url": url.toString(), "method": "POST"});
	if(!currentCheck) {
		console.error("Assertion failed:", "response.status.toString() \u003D\u003D\u003D \"200\"");
		fail()
	};

	vars['accessToken'] = jsonpath.query(response.json(), '$.token')[0];
	
	console.log("Starting request to http://example.org/profile...");
	try {
		url = new URL('http://example.org/profile');
	} catch(e) {
		console.error("Invalid URL: http://example.org/profile");
		fail()
	}
	

	body = null;
	
	response = http.request('GET', url.toString(), body, {
		// TODO(mem): build params out of options for the check
		tags: {
		  name: '1', // TODO(mem): give the user some control over this?
		  __raw_url__: 'http://example.org/profile',
		},
		redirects: 0,
		headers: {"Authorization":'Bearer '+vars['accessToken']}
	});
	console.log("Response received from http://example.org/profile, status", response.status);
	if(logResponse) {
		const body = response.body || ''
		console.log("Response body received from http://example.org/profile:", body.slice(0, 1000));
	}
	if(response.error) {
		console.error("Request error:" + url.toString() + ": " + response.error)
	}
	
}
