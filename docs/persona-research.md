# Bosun persona research

Bosun uses short Polish replies that combine dry irony, maritime language, and direct criticism of the current message or effort.

The static catalog contains 6,400 original replies. The generator combines one contextual lead with one dependent finisher and copies no source text.

## Research sources

- [LingVaria: incongruity and punchlines](https://journals.akademicka.pl/lv/article/view/6023) informed the use of a clear semantic turn near the end.
- [Prace Językoznawcze: punchline construction](https://czasopisma.uwm.edu.pl/index.php/pj/article/view/12286) informed concision, wordplay, meaning shifts, and sudden perspective changes.
- [Folia Linguistica: syntax in humorous texts](https://czasopisma.uni.lodz.pl/linguistica/article/view/1332) informed short parallel structures, pauses, and controlled repetition.
- [Politeja: ironic deprecation](https://journals.akademicka.pl/politeja/article/download/3476/3045/4137) informed indirect negative judgment through apparent praise and contrast.
- [Culture.pl: Polish humor](https://culture.pl/pl/superartykul/smieje-sie-wiec-jestem-osiem-mistrzyn-polskiego-humoru) informed the balance between aggressive satire, irony, and self-aware distance.
- [Narodowe Centrum Kultury: sardonic humor](https://nck.pl/projekty-kulturalne/projekty/ojczysty-dodaj-do-ulubionych/ciekawostki-jezykowe/SARDONICZNY_smiech) informed the bitter, ironic, mocking tone.
- [Discord Community Guidelines](https://discord.com/guidelines) informed the exclusions for threats, private data, sexual abuse, self-harm attacks, and protected-trait attacks.

## Writing rules

- Write every reply in Polish.
- Write exactly one dash-linked sentence without final punctuation.
- Keep each reply self-contained and suitable for a random response.
- Direct criticism at the message, prompt, effort, logic, plan, or repeated behavior.
- Put the strongest image, reversal, or punch word near the end.
- Use category-specific finishers instead of unrelated reaction scenes.
- Use maritime details to support the Bosun persona.
- Permit Polish profanity in the harsh and savage tiers.
- Exclude protected-trait attacks, threats, private data, sexual abuse, and self-harm attacks.
- Do not copy a published joke, quotation, or source phrase.

## Escalation tiers

- Tier 1, `sharp`, contains 2,140 dry and corrective replies.
- Tier 2, `harsh`, contains 2,130 stronger replies for the first two triggers.
- Tier 3, `savage`, contains 2,130 profane replies from the third trigger.

The selector counts triggers in a rolling ten-minute window. It excludes each user's 20 most recent reply identifiers from the next choice.

## Message categories

The local classifier selects 19 types. These types include social checks, status checks, opinions, stances, reactions, confusion, and acknowledgements.

Each non-generic category contains 100 replies in each spice tier. The generic category keeps the original 1,000 replies.

The classifier ignores links, platform tags, code blocks, and custom emoji text. This rule reduces false category matches from embedded data.

## Dynamic reply profile

The local profile uses only fixed labels. It describes the trigger, category, conversation role, question kind, response mode, form, tone, and spice.

The profile can identify happy, angry, joking, sad, or neutral tone. It never contains the original words.

This profile gives a model useful context without message content. It cannot preserve exact topics or personal references.

For each event, the selector supplies eight vetted static replies from the catalog. Candidate zero remains the static fallback.

The selector reserves the batch until the decision completes. It records only the committed candidate as a recent reply.

Jev returns one typed candidate choice with probabilities and confidence. The bot displays only the selected catalog text and never displays model text.
