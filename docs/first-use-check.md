# Three first-use journeys

On November 22, 2026, one tester simulated three people with different goals. Each started on healthctl.com, followed the install link, then used a separate browser session and an empty local journal. Each journal had access to the same authorized ChatGPT plan. The tester entered fictional data, approved disclosures, and used ordinary browser controls. Downloads were inspected after the visits. Desktop installation and provider sign-in were outside these three tasks.

| Goal | What happened on the original build | Change |
| --- | --- | --- |
| Describe a sore knee, pause and return, then correct the history | After the user described pain from kneeling, the model asked about bearing weight and later about swelling. During the first model wait, the journal showed only “Interview in progress.” A correction about hard ground appeared in the confirmed brief. | A pending journal entry now says “Waiting for model.” The journal updates while it stays open. |
| Understand a routine urine result | The user reviewed and approved one printed result from a fictional PDF. Six different buttons had the same accessible name. The interview asked when symptoms had started before the user had reported any. After the user explained this was routine, the final assessment said there was insufficient information. | Each review button includes the source location in its accessible name. The timing question now first asks whether there were any symptoms. |
| On a phone-sized screen, bring a saved headache episode into a short visit | After saving the episode, the user had to go back to the journal, discover “Start a return visit,” and find the entry again. The short interview used two follow-up answers, and model processing took additional time. The episode survived in the confirmed brief. | A diary entry now offers “Start a visit with this episode.” The intake selects that entry and still requires explicit permission to send it. The short option describes its answer limit. |

All three original visits produced a clinician brief and a JSON download with the confirmed revision and a completed GPT-6.2-Sol assessment. The second visit recorded insufficient information. The other two returned provisional explanations. The first model-processing intervals after submitted answers were approximately 118, 108, and 121 seconds respectively, measured from saved answer and turn timestamps. These timings describe these runs on this plan.

A separate empty-installation check found that a first-time user without a model had to enter provider settings to discover offline practice. An unconfigured journal now offers “Try the offline demo” directly.

These were simulated usability sessions with fictional histories. Independent first-time users have yet to test these changes.

The three tasks were repeated in new journals with the updated interface and live plan. All three exported a confirmed history and a completed assessment using the updated interview prompt. The reopened journal row cleared its waiting label without a reload after 115 seconds. The report-first retest offered a neutral symptom question and returned insufficient information for the routine result. The diary retest preselected only the chosen episode, required the user to approve sending it, and included it in the exported visit. No reduction in model inference time was established.

The automated browser regression suite passed 48 desktop and mobile tests. The local verification also checked the printed reports, the direct offline-demo entry, and playback and captions for both website videos.
