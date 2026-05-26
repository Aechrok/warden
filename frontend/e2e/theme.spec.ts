import { test, expect } from '@playwright/test'

test('theme toggle switches dark/light', async ({ page }) => {
  await page.goto('/login')
  const html = page.locator('html')
  // Default: system preference — just verify toggle button exists.
  const toggle = page.locator('[aria-label="Toggle theme"]')
  await expect(toggle).toBeVisible()
  const before = await html.getAttribute('data-theme')
  await toggle.click()
  const after = await html.getAttribute('data-theme')
  expect(before).not.toEqual(after)
})
