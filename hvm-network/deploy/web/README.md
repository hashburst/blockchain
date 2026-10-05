# Website publication preparation

These are staged static pages, not files deployed to a VPS.

- `hvm-network.en.html`: English HVM Network content for integration into
  blockchainapi.one/hashburst. Preserve the existing dashboard and API routes;
  this standalone page is not a drop-in replacement for its unseen application.
- `hashburst.io.html`: English/Italian landing page with in-page language links.

No external fonts, analytics, scripts, secrets, wallet connection or write RPC.
The noindex directive is deliberate for staged pages.

Publication follows RC3-PREPARATION.md gates. Before deployment, inspect the
current document roots and page entry points, back up the exact affected files,
review content against the accepted mainnet manifest, update status/date and
release links, remove staging noindex, and check both language sections.
Use a temporary file in the destination directory then atomic rename for each
static page. Preserve nginx routing and all network/storage services. Check HTTP
status, language, content and API routing after publication; rollback only the
website files if those checks fail, never blockchain state or signing journals.

No deployment command is provided before the gates and live page paths are verified.
