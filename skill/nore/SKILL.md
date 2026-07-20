---
name: nore
description: Route natural-language Nore blog requests to the matching Nore CLI behavior. Use when the user mentions a Nore blog, its articles or publishing history, or asks to publish or release a Nore blog.
---

# Route Nore blog requests

Translate each user intent into exactly one primary Nore CLI behavior:

- When the user wants to see, browse, or search a Nore blog or its articles, use the post-listing behavior: `nore post list`.
- When the user asks about publication history, a previous publication, or recent publication status, use the release-history behavior: `nore release list`.
- When the user asks to publish, release, or update a Nore blog, use the release-creation behavior: `nore release create`.

Choose by the requested outcome: performing a publication maps to release creation; reviewing what happened maps to release history; otherwise a Nore blog or article request maps to post listing.

Use the selected command's help to resolve required arguments and options, then run it with JSON output. Do not expand one intent into additional Nore CLI behaviors.

JSON commands cannot prompt for a site. If the user did not identify a site, use `nore site list --json` as a preparatory lookup, then pass the selected UUID or ident with `--site` to the primary command. This lookup does not change which behavior is primary. If multiple sites are available and the user's intent does not identify one, ask the user which site to use.
