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
