# User guide

healthctl helps you prepare for a healthcare visit. It takes your history in your own words, checks it for urgent warning signs, lets you correct it, and prints a brief for your clinician. It is beta software. Nothing in it has been clinically reviewed, it can be wrong, and it never replaces a clinician. For a possible emergency, seek local emergency care immediately.

## Start an encounter

Choose **New encounter**, describe what is bothering you, and answer the two intake questions. The app screens everything you write, at every step, for urgent signs. If it finds one it shows a warning straight away. It never waits for the model.

## The interview

Some questions come from the app and some from the model you connected.

- **The app's own questions** cover what every concern of your kind needs: when it started, how it has changed, how much it affects you, and safety questions such as "Have you coughed up any blood?". A yes to a safety question shows the emergency warning at once.
- **The model's questions** follow your story. The app writes the wording of most of them and checks every one. **Why this question?** is always the app's explanation. Where the model added its own note, it is labeled as the model's suggestion.

Answer in your own words or pick a choice. Choices never name a symptom you haven't mentioned. You can stop at any time with **Review my history**; anything the app still expected is listed under **Not yet asked**, and goes into your brief.

## Warnings

- **Seek emergency care** interrupts the interview. It stays until you correct the words it came from. Several urgent phrases share one warning that lists each of them.
- **Get this checked promptly** stays visible and lets you continue.

The model can raise a warning, but it can never clear one.

## Review and confirm

Check every finding against what you said, correct anything that is wrong, and answer the quick safety question. Then confirm the history. Any safety question still unanswered is shown on this screen.

## The optional assessment

After you confirm, you can ask for a provisional AI assessment. It is not a diagnosis. Possibilities on the app's condition list use the app's names and descriptions; any other name is labeled as the model's own suggestion. For some concerns the app adds **Ask your clinician about** lines for dangerous causes the assessment left out. You can ask for the assessment to run twice; possibilities that change between the runs are marked uncertain, and the assessment says whether the second run happened. If the second run fails, it says so, and you should treat every possibility as uncertain.

## Briefs, exports and your account

The clinician brief lists your history, warnings, open questions and anything not yet asked. Viewing the report on screen never asks for your password. Downloading it, sharing, deleting and changing where your history is sent ask for your password if you signed in more than ten minutes ago. Sessions end after an hour without activity, and **End every session** signs you out on every device at once.

## Forgot your password?

There's no email reset, because healthctl runs on your own computer. Reset the password on the computer that runs it. In the Mac or Windows app, open the healthctl launcher and choose **Reset owner password…**. For a Docker installation, run `python3 scripts/maintenance.py recover-owner` in the healthctl folder. The sign-in page and the password prompt show the same steps under **Forgot your password?**

A reset signs you out on every device, ends shared brief links and disconnects ChatGPT accounts, so connect ChatGPT again afterwards. Your encounters, diary and uploads are kept.

## Keeping your records

On the desktop apps, choose **Back up now** regularly and keep the backup passphrase safe. A Docker installation backs up on a schedule; the journal shows when the last backup succeeded. See the [operator guide](operator-guide.md) for the details.
