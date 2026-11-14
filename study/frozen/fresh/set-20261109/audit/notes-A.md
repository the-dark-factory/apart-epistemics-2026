# Notes A (TY2024 Form 1040, worked from IRS text sources only)

Common: Tax Table = i1040gi p.64+ (all TI < 100,000, no qualified dividends, so no QDCG worksheet, no Tax Computation Worksheet). Std deduction: single 14,600 / MFJ 29,200 / HOH 21,900; +1,950 (single/HOH) if born before 1/2/1960. No return reaches Schedule 2 or Form 8959 (wages far below 200k, no SE, no other taxes). Schedule B not needed where interest/dividends <= 1,500 (f0069, f0086). Filing status taken exactly as given in input. SSNs in the data begin with 9 (look like ITIN range); treated as valid SSNs because has_ssn is true.

## f0065 (single, b.1960-10-19)
- L12: 14,600. Born Oct 1960, so not "born before Jan 2, 1960"; no 65+ addition.
- L10: student loan interest 2,500. Worksheet: line 4 (AGI before deduction) 80,000 is not more than 80,000, so line 8 = 0, deduction 2,500. AGI 77,500; TI 62,900.
- L16: Tax Table row 62,900-62,950, single = 8,897.
- L19: 0 (no dependents). L24: 8,897. L25d: W-2 4,800. L27/L28: 0 (AGI > 18,591; no children). Owed 4,097.

## f0069 (MFJ, 3 children b.2019, 2017, 2013)
- L9: wages 30,014 + 13,567 = 43,581, + ordinary dividends 445 (qualified 0) = 44,026. L12: 29,200. TI 14,826.
- L16: Tax Table row 14,800-14,850, MFJ = 1,483.
- L19: Sch 8812: 3 children under 17 (all ages 5, 7, 11) x 2,000 = 6,000; no phaseout. Credit Limit WS A: line 18 (1,483) less Sch 3 items (0) = 1,483. L14 = min(6,000, 1,483) = 1,483.
- L24: 1,483 - 1,483 = 0. L25d: 3,601 + 1,221 = 4,822.
- L27: Step 1: AGI 44,026 < 66,819 ok; investment income 445 < 11,600. Worksheet A: line 1 earned income 43,581 -> table row 43,550-43,600 MFJ/3 = 4,895; line 3 AGI 44,026 differs and >= 29,640 so line 5 = table row 44,000-44,050 MFJ/3 = 4,801; smaller = 4,801.
- L28: 8812 line 16a = 6,000 - 1,483 = 4,517; 16b = 3 x 1,700 = 5,100; line 17 = 4,517; line 18a 43,581, line 19 = 41,081, line 20 = 15% = 6,162; 16b >= 5,100 and line 20 >= line 17, so skip Part II-B, ACTC = 4,517.
- L33 = 4,822 + 4,801 + 4,517 = 14,140. Refund 14,140. (Spouse box for presidential fund noted; no tax effect.)

## f0080 (single, 1 son b.2008-10-03)
- Judgement: filing status single as given (he would likely qualify for HOH, but input says single; not changed).
- L12: 14,600. TI 31,409. L16: row 31,400-31,450 single = 3,539.
- L19: son is 16 at end of 2024 (under 17): CTC 2,000. Limit WS A = 3,539, so L19 = 2,000. L24 = 1,539.
- L28: 8812 line 16a = 0, so no ACTC.
- L25d: 3,220. L27: AGI 46,009 < 49,084 (1 child); EI = AGI so Worksheet A line 6 = line 2 = table row 46,000-46,050 single/1 = 489.
- Payments 3,709; refund 2,170.

## f0086 (MFJ, no dependents; W-2 8,421 taxpayer; 1099-G 8,565 spouse; 1099-INT 809 spouse)
- L9: 8,421 + 809 (L2b) + 8,565 (Sch 1 line 7 via L8) = 17,795. L12 29,200 so TI 0, L16 0. No credits.
- L25d: W-2 926 + 1099-INT box 4 521 + 1099-G 0 = 1,447. L26: 857 + 2,922 + 822 + 2,833 = 7,434.
- L27: no children. Step 4: ages 51/50 ok; investment income 809 < 11,600; AGI 17,795 < 25,511. Earned income = wages only 8,421 (unemployment is not earned income). WS A: line 2 = row 8,400-8,450 MFJ/0 = 632; line 3 AGI 17,795 differs; line 5 test: AGI not < 17,250 so look up 17,795, row 17,750-17,800 MFJ/0 = 592; smaller = 592.
- L33 = 1,447 + 7,434 + 592 = 9,473; refund 9,473. (1099-INT withholding of 521 on 809 interest taken as given.)

## f0091 (HOH, wages 4,987, educator expenses 136)  -- AMBIGUOUS EIC
- L10: educator expenses 136 (Sch 1 line 11, limit 300). AGI 4,851. L12 21,900. TI 0, L16 0, L24 0.
- L19: 0. Only a "qualifying person" name and SSN given; no DOB, relationship or months lived; hoh_planning_to_claim_dependent flag absent (not claimed as dependent), so no CTC/ODC.
- L25d: 748.
- L27 judgement: no qualifying-child facts in the data, so I computed EIC as a filer without a qualifying child (age 44; AGI 4,851 < 18,591): WS A line 2 = table row 4,950-5,000 (EI 4,987) = 381; AGI differs, AGI < 10,330 so line 6 = 381. If the person is in fact an EIC qualifying child (1 child), EIC would be table row 4,950-5,000 single/HOH 1 = 1,692, giving payments 2,440 and refund 2,440. This is a guess flagged, not a determination.
- L28: 0.

## f0100 (single, b.1999-09-12, W-2 27,198, 1099-G 9,246)
- L9: 27,198 + 9,246 = 36,444. L12 14,600. TI 21,844. L16 row 21,800-21,850 single = 2,387.
- L25d: 4,351 (1099-G withholding 0). L27: age 25 ok but AGI 36,444 > 18,591, no EIC. Refund 1,964.

## f0102 (single, W-2 47,470, two 1099-INT 2,895 + 2,907)
- L2b 5,802 (Schedule B required, > 1,500). L9 53,272. L12 14,600. TI 38,672. L16 row 38,650-38,700 single = 4,409.
- L25d: W-2 4,747 + 521 + 521 = 5,789. No EIC (AGI > 18,591). Refund 1,380.

## f0111 (single, b.1951, two W-2s 52,129 + 3,879)
- L12: 14,600 + 1,950 (65+) = 16,550. L9 56,008. TI 39,458. L16 row 39,450-39,500 single = 4,505. Box 12 DD is informational.
- L25d: 3,649 + 620 = 4,269. No EIC. Owed 236.

## f0118 (HOH, 2 sons b.2020-04-07 and b.2008-07-08, wages 55,768)
- Input also carries spouse name/DOB (and spouse signature fields) though status is HOH: followed the stated status HOH and flagged. Spouse has no SSN in the data.
- L12: 21,900. TI 33,868. L16: row 33,850-33,900 HOH = 3,734.
- L19: both sons under 17 -> 4,000; limit WS A = 3,734; L19 = 3,734. L24 = 0.
- L28: 16a = 4,000 - 3,734 = 266; 16b = 2 x 1,700 = 3,400; 17 = 266; line 20 = 15% x (55,768 - 2,500) = 7,990; 16b < 5,100 so ACTC = min(266, 7,990) = 266.
- L27: Step 1 requires AGI less than 55,768 for 2 children; AGI is exactly 55,768, so no EIC (table note also says 55,768 or more cannot take credit).
- L25d: 5,576. L33 = 5,576 + 266 = 5,842. Refund 5,842.

## f0138 (single, b.1957, W-2 18,187, 1099-INT 2,485, educator expenses 81)
- L9: 18,187 + 2,485 = 20,672; L10 = 81 (educator); AGI 20,591. L12 = 14,600 + 1,950 = 16,550. TI 4,041. L16 row 4,000-4,050 = 403.
- L25d: 1,818 + 521 = 2,339. No EIC (age 67 and AGI > 18,591). Refund 1,936.
