\set ON_ERROR_STOP on
BEGIN;

-- Internal note (root-cause + verification trail)
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id,
$$Internal (claude-triage 2026-06-23): Root-caused to the multi-word OpenFDA search bug fixed in commit 8c8c6ab (2026-05-12 — the SAME day Holly reported this). Both in-app med-detail paths (add-form lookupDrugInfo + edit-form loadAdditionalDrugInfo in templates/medications.html) call /api/drugs/info LIVE -> fetchFromOpenFDA, which now quotes the full drug name as a phrase (with first-token fallback). Pre-fix, unquoted multi-word search matched loose tokens and limit=1 returned an unrelated pseudoephedrine "Hcl" label; that is exactly Holly's "most going to pseudoephedrine" symptom. 8c8c6ab is an ancestor of HEAD and shipped to prod via subsequent deploys (latest = onboarding deploy 2026-06-18). Verified against api.fda.gov: fluoxetine, fluoxetine hcl, risperidone, clonidine, guanfacine/Intuniv, sertraline/Zoloft, Abilify, Risperdal, Focalin, Prozac, Adderall, Concerta, Aripiprazole all resolve correctly now. NO new code change required for the core bug. Set waiting_on_user pending Holly's confirmation on Matty's actual meds (prod med list not visible from support DSN, so cannot self-verify her exact data per resolution protocol). RESIDUAL (separate, low severity): bare "Melatonin" matches a homeopathic combo product (GUNA-GERIATRICS) via limit=1 since there is no single official FDA melatonin label — flagged to Bryan as a separate data-quality item, out of scope here.$$,
true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM support_tickets WHERE ticket_number = 112386;

-- Public reply to Holly
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id,
$$Hi Holly — thank you for catching this, and you were right: medications were pulling their details (and the source link) from the wrong drug. The root cause was in how we searched the FDA database — for any medication whose name is more than one word, our search was matching loose word fragments and could land on an unrelated product, which is why so many of Matty's meds were resolving to pseudoephedrine.

We've fixed the search so it matches the full medication name as a unit, and that fix is now live in production. We re-tested a broad set of common medications (fluoxetine, risperidone, guanfacine/Intuniv, sertraline/Zoloft, Abilify, Adderall, Concerta and others) and they now resolve to the correct drug and the correct source.

Could you open a couple of Matty's medications and check that the "Uses" section now reads correctly? If any specific one still shows the wrong details, just reply with that medication's exact name as you entered it and we'll dig into that one directly. (One known exception: plain supplements like melatonin don't have a single official FDA label, so those can still look a little odd — we're tracking that separately.)$$,
false, 'support@mycarecompanion.net', 'CareCompanion', 'Support'
FROM support_tickets WHERE ticket_number = 112386;

UPDATE support_tickets SET status = 'waiting_on_user', updated_at = now()
WHERE ticket_number = 112386 AND status <> 'waiting_on_user';

SELECT ticket_number, status FROM support_tickets WHERE ticket_number = 112386;
COMMIT;
