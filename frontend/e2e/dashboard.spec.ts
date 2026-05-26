import { test, expect } from '@playwright/test'

// This test requires a running full dev stack (Warden server + Postgres).
// Skip unless PLAYWRIGHT_BASE_URL is set.
test.skip(
  !process.env.PLAYWRIGHT_BASE_URL,
  'Requires full dev stack (PLAYWRIGHT_BASE_URL not set)',
)

test('dashboard loads stat cards', async ({ page }) => {
  await page.goto('/dashboard')
  // Expect at least one stat card to be present.
  await expect(page.locator('[data-testid="stat-card"]').first()).toBeVisible()
})
