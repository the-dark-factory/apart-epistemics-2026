# Notes B (TY2024 Form 1040, by hand from IRS sources)
Tax Table = i1040gi p.64ff; EIC Table = i1040gi; 8812 per f1040s8 / i1040s8 Credit Limit Worksheet A (no Sch 3 credits in any return). No Sch 2 / Form 8959 needed in any return (wages < 200k, no SE, no other taxes).

## f0011 (HoH, DOB 1967, no dependents listed)
12: HoH std 21,900, no age/blind add. 16: no qualified divs (QD=0) so Tax Table HoH 59,400-59,450 = 6,800. 19: none. 24: 6,800. 25d: W-2 6,697. 26: est. pmts 2,482+2,940+2,228+2,433 = 10,083. 27/28: none (no child; AGI 81,334 over no-child EIC limit). Payments 16,780; refund 9,980.
Judgement: filing status HoH given but no qualifying person/dependent in data, and spouse name/DOB present. Took HoH as stated; flagged as ambiguous. Ordinary dividends 6,920 (line 3b).

## f0012 (single, daughter b. 2021)
12: 14,600. 15: 1,400. 16: Tax Table 1,400-1,425 single = 141. 19: 8812 line12 2,000, line13 141 -> 141. 24: 0. 25d: 2,240. 27: EIC Table single 1 child, 16,000-16,050 = 4,213 (EI = AGI so one lookup). 28: 16a 1,859, 16b 1,700, line 20 = 15% x (16,000-2,500) = 2,025 -> 1,700. 32 = 5,913; 33 = 8,153.

## f0018 (MFJ, daughter b. 2021)
1a 27,494+24,051. 12: 29,200. 15: 22,345. 16: Tax Table MFJ 22,300-22,350 = 2,233. 19: 2,000 (limit 2,233). 24: 233. 25d: 1,649+3,607 = 5,256. 27: EIC Table MFJ 1 child 51,500-51,550 = 716. 28: 0 (credit fully used). Refund 5,739.

## f0023 (MFJ, ages 35/32, no children)
Income: wages 13,318 + interest 4,334 = 17,652. 12: 29,200; taxable 0; tax 0. 25d: 366+1,157+521 (1099-INT backup withholding) = 2,044. 27: Worksheet A. EI 13,318 -> table MFJ 0 kids 13,300-13,350 = 632; AGI 17,652 is not < 17,250 so look up AGI too: 17,650-17,700 = 599; smaller = 599. Investment income 4,334 <= 11,600. Age 25-64 met.

## f0046 (single, student loan interest)
Student loan worksheet: MAGI 80,001; (80,001-80,000)/15,000 = .000 (3 places) -> no reduction; 10 = 2,500 (even unrounded 2,499.83 -> 2,500). AGI 77,501; 12: 14,600; 15: 62,901; 16: Tax Table single 62,900-62,950 = 8,897. 25d 9,600. Refund 703.

## f0084 (single, daughter b. 2007-09-15 = age 17 at 12/31/2024)
Income: wages 68,380 + div 1,373 + 1099-G unemployment 9,358 = 79,111. 12: 14,600; 15: 64,511. 16: QDCG worksheet: L1 64,511, L4 686, L5 63,825, L7/L8 47,025, L9 0, L12 686, L17 686, L18 = 15% = 103 (102.9 rounded), L22 Tax Table 63,800-63,850 = 9,095, L23 9,198, L24 Tax Table 64,500-64,550 = 9,249; smaller 9,198. 19: age 17 -> not CTC; ODC $500 (8812 line 6 = 1) = 500. 24: 8,698. 25d: 10,257 W-2 + 935 1099-G = 11,192. 27: none (AGI above limit). 28: none (credit fully used). Refund 2,494.

## f0101 (single)
Income 57,519 + 4,106 (all qualified) = 61,625. 15: 47,025. QDCG: L5 = 42,919; L9 = 4,106 at 0%; L22 Tax Table 42,900-42,950 = 4,919; L24 (47,000-47,050) 5,411; smaller 4,919. 25d 8,052. Refund 3,133.

## f0105 (MFJ, two children b. 2017-04-13 and 2008-05-01 -> 16 at year end, both CTC)
Income 87,985 + interest 5,837 + div 1,686 = 95,508. Sch 1 line 21 student loan interest 670 (MAGI far below 165,000). AGI 94,838. 12: 29,200; 15: 65,638. 16: QDCG L5 = 64,545 (qualified 1,093), Tax Table MFJ 64,500-64,550 = 7,279 vs L24 65,600-65,650 = 7,411 -> 7,279. 19: 4,000 (limit 7,279). 24: 3,279. 25d: 3,316+7,519+521+521 = 11,877. 27/28: 0 (EIC AGI too high; CTC fully nonrefundable). Refund 8,598. W-2 box 12 DD ignored (informational).

## f0129 (HoH, DOB 1951-10-23 = 73)
Income 24,112 + interest 4,324 = 28,436. 12: HoH 21,900 + 1,950 (65+, unmarried) = 23,850. 15: 4,586. 16: Tax Table HoH 4,550-4,600 = 458. 25d: 1,928+521 = 2,449. 
Judgement/ambiguity: only a qualifying child name + SSN is given (header line), no DOB, relationship, months lived, and no dependent_detail; hoh_planning_to_claim_child_or_dependent_credit = false, and the 1040 header name field is for a child who is NOT the taxpayer's dependent. So 19 = 0. EIC (27) set to 0 because child age/residency/relationship cannot be tested -- NOT verified; if the child were an EIC qualifying child the credit would be material (AGI 28,436).

## f0132 (single, daughter b. 2016-04-14)
Wages 3,859+10,393 = 14,252. 12: 14,600; taxable 0; tax 0; 19: 0 (limited by zero tax). 25d: 347+727 = 1,074. 27: EIC Table single 1 child 14,250-14,300 = 4,213. 28: 16a 2,000; 16b 1,700; line 20 = 15% x 11,752 = 1,763 -> 1,700. Payments 6,987.
Note: sp_dob 1991-01-01 present in f0084/f0132 with status single: treated as placeholder, ignored.
