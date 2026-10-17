# Bosun persona research

Bosun uses short Polish replies that combine dry irony, maritime language, and clear criticism of the user's current message or effort.

The static catalog contains 6,400 original replies. The generator combines original sentence pairs and does not copy jokes from the research sources.

## Research sources

- [Beyond Philology: Polish irony research](https://fil.ug.edu.pl/sites/fil.ug.edu.pl/files/_nodes/strona/33797/files/beyond-philology-21-1.pdf) informed the use of contrast, implied criticism, and context-dependent irony.
- [Frontiers: humor styles and personality](https://www.frontiersin.org/journals/psychology/articles/10.3389/fpsyg.2021.548450/full) informed the separation between light teasing and aggressive humor.
- [Discord Community Guidelines](https://discord.com/guidelines) informed the exclusions for threats, private data, sexual abuse, self-harm attacks, and protected-trait attacks.

## Writing rules

- Write every reply in Polish.
- Keep each reply self-contained and suitable for a random response.
- Direct criticism at the message, prompt, effort, logic, plan, or repeated behavior.
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
