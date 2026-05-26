import { test, expect } from '@playwright/test'

test('login page shows SSO button', async ({ page }) => {
  await page.goto('/')
  // Should redirect to /login if not authenticated.
  await expect(page).toHaveURL(/\/login/)
  await expect(page.getByText('Sign in with SSO')).toBeVisible()
})
