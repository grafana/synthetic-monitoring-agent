import { check } from 'k6'
import { browser } from 'k6/browser'
import secrets from 'k6/secrets'

export const options = {
	scenarios: {
		ui: {
			options: {
				browser: {
					type: 'chromium',
				},
			},
		},
	},
};

export default async function() {
	const my_secret = await secrets.get('secret_name');
	const context = await browser.newContext();
	const page = await context.newPage();

	try {
		await page.goto("https://test.k6.io/my_messages.php");

		await page.locator('input[name="login"]').type("admin");
		await page.locator('input[name="password"]').type(my_secret);

		await Promise.all([
			page.waitForNavigation(),
			page.locator('input[type="submit"]').click(),
		]);

		await check(page.locator("h2"), {
			header: async (locator) => (await locator.textContent()) == "Welcome, admin!",
		});
	} catch (e) {
		console.log('Error during execution:', e);
		throw e;
	} finally {
		await page.close();
	}
}
