# Recording the launch demo

The launch clip should be shorter than 30 seconds and show one complete emotional arc: create a switch, miss the accelerated deadline, open the release email, enter enough recovery shares, and reveal the message locally.

## Prepare

1. Install Docker, Node.js 24+, and Playwright's Chromium browser.
2. Use a 1440 x 900 browser viewport at 100% zoom and hide bookmarks/extensions.
3. Run `TIME_SCALE=86400 KEEP_DEMO_RUNNING=1 make demo`.
4. Open the dashboard at `http://localhost:5173` and Mailpit at `http://localhost:8025`.
5. Use fictional names, addresses, and content. Never record real recovery shares.

## Shot list

1. Create “Family letters” with a seven-day interval and three-day grace period.
2. Add a short encrypted message and a 2-of-3 trustee set.
3. Save two Recovery Kits outside Afterwords.
4. Let the countdown accelerate from safe to overdue and then released.
5. Open the calm recipient email in Mailpit.
6. Paste two demo shares and reveal the message in the recipient page.
7. End on the zero-knowledge statement, not a terminal window.

Record at 30 fps, trim idle frames, and export both MP4 and an optimized GIF. Keep the GIF under roughly 8 MB for README performance. Verify the final clip in light and dark GitHub themes before publishing it.
