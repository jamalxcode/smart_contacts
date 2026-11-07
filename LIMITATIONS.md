# What Smart Contacts won't do, and what's next

Smart Contacts keeps every contact on this Mac. These are the deliberate limits, plus the ideas
that haven't been built yet.

## Won't be built (privacy)

- **Profiling people:** searching Google, X, LinkedIn or other sites for every contact and
  compiling what each person does, which groups they belong to or what they're involved in.
- **Reverse lookups:** finding someone's identity or social accounts from their phone number or
  email. That depends on data brokers and scraping, which LinkedIn and X forbid.
- **Background tracking:** watching people's online activity over time.
- **Online searches of contacts:** no LinkedIn search, social media search, X search or Google
  search of contacts from inside the app. Profile links are added by hand under
  *Edit → Web & Social*.

## Possible, but left out to keep contacts private

- **Reading your messages and calls:** scanning Messages, WhatsApp, Mail or call history to work
  out who you talk to most, or to fill in "last contacted" automatically.
- **Fetching contact photos online:** from Gravatar, WhatsApp or similar (it sends emails and
  numbers to outside servers).
- **Placing contacts on a map:** turning addresses into map locations needs an online service.
- **Checking numbers with a carrier:** confirming a number is live or which network it's on.
- **Asking AI about the whole address book:** questions like "who do I know in banking?" would
  send every contact to a cloud model.
- **Cloud backup or sync of contacts:** iCloud, Google or any server.

## Possible, but risky to automate

- **Guessing relationships:** same surname = family, same address = household. Too often wrong;
  only same-company links are automatic.
- **Ranking people:** "importance" or relationship-strength scores.
- **Inferring sensitive traits** (religion, health, nationality) from names or notes.
- **Matching faces** of contacts in your photos.
- **Messaging contacts on your behalf:** automatic birthday wishes or bulk WhatsApp sent without
  you reviewing each one.
- **Sharing your network map:** exporting who-knows-whom gives away other people's relationships.

## Current limits on contact data (could be lifted)

- Contacts live on one Mac only; there is no conflict handling if the file is edited elsewhere.
- Contact data has no encryption of its own; it relies on FileVault.
- Connections between people are not included in vCard exports.
- Search needs the exact spelling (no typo tolerance) — Arabic letter forms and accents are matched.
- Google Contacts come in only through a .vcf export, not a direct import.
- Contact photos: iPhone HEIC needs converting to JPEG; photos are shrunk to 512 px.
- Undoing an edit to a contact isn't possible; only deleted contacts can be restored (Trash, 30 days).

## Not built yet (could be)

- Organisation, "how I know them" and "met at" fields you fill in yourself, linked into connections
- A quick search from the menu bar
- An encrypted private backup of your contacts (never to this public repo)
- Birthday reminders as Mac notifications
- Editing several contacts at once (adding a group to many people, for example)

## Done

- **Six degrees of separation:** link people as friend, family, colleague… (⌘K); *How Are They
  Connected?* (⌘⇧K) finds the shortest chain between two people, up to six steps. Built offline,
  from your own contacts only.
