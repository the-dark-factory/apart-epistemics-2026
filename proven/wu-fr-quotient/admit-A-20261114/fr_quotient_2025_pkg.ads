with Fr_Bareme_2025_Pkg;
package Fr_Quotient_2025_Pkg with SPARK_Mode, Pure is
  -- PURPOSE: French quotient familial 2025 with its cap (1 791 per extra half-part; 4 224 for case T), in half-cents.
  -- USAGE: call Tax_Half_Cents (R, Couple, C, Alone) with R net taxable euros, C children 0 .. 10.
  use type Fr_Bareme_2025_Pkg.Wide;
  use type Fr_Bareme_2025_Pkg.Half_Parts;

  type Children is range 0 .. 10;

  function Higher (A, B : Fr_Bareme_2025_Pkg.Wide) return Fr_Bareme_2025_Pkg.Wide is
    (if A > B then A else B);

  function Case_T (Couple : Boolean; C : Children; Alone : Boolean) return Boolean is
    (not Couple and then Alone and then C >= 1);

  function Base_Half_Parts (Couple : Boolean) return Fr_Bareme_2025_Pkg.Half_Parts is
    (if Couple then 4 else 2);

  function Extra_Half_Parts (Couple : Boolean; C : Children; Alone : Boolean) return Fr_Bareme_2025_Pkg.Wide is
    (if C = 0 then 0 elsif C = 1 then 1 + (if Case_T (Couple, C, Alone) then 1 else 0) else 2 * Fr_Bareme_2025_Pkg.Wide (C) - 2 + (if Case_T (Couple, C, Alone) then 1 else 0));

  function Total_Half_Parts (Couple : Boolean; C : Children; Alone : Boolean) return Fr_Bareme_2025_Pkg.Half_Parts is
    (Fr_Bareme_2025_Pkg.Half_Parts (Fr_Bareme_2025_Pkg.Wide (Base_Half_Parts (Couple)) + Extra_Half_Parts (Couple, C, Alone)));

  function Cap_Allowance (Couple : Boolean; C : Children; Alone : Boolean) return Fr_Bareme_2025_Pkg.Wide is
    (if Case_T (Couple, C, Alone) then 844_800 + 358_200 * (Extra_Half_Parts (Couple, C, Alone) - 2) else 358_200 * Extra_Half_Parts (Couple, C, Alone));

  function Uncapped (R : Fr_Bareme_2025_Pkg.Euros; Couple : Boolean; C : Children; Alone : Boolean) return Fr_Bareme_2025_Pkg.Wide is
    (Fr_Bareme_2025_Pkg.Bareme_Half_Cents (R, Total_Half_Parts (Couple, C, Alone)));

  function Base_Tax (R : Fr_Bareme_2025_Pkg.Euros; Couple : Boolean) return Fr_Bareme_2025_Pkg.Wide is
    (Fr_Bareme_2025_Pkg.Bareme_Half_Cents (R, Base_Half_Parts (Couple)));

  function Tax_Half_Cents (R : Fr_Bareme_2025_Pkg.Euros; Couple : Boolean; C : Children; Alone : Boolean) return Fr_Bareme_2025_Pkg.Wide is
    (Higher (Uncapped (R, Couple, C, Alone), Base_Tax (R, Couple) - Cap_Allowance (Couple, C, Alone)));

  function Never_Below_Uncapped (R : Fr_Bareme_2025_Pkg.Euros; Couple : Boolean; C : Children; Alone : Boolean) return Boolean is
    (Tax_Half_Cents (R, Couple, C, Alone) >= Uncapped (R, Couple, C, Alone))
    with Post => Never_Below_Uncapped'Result;

  function Saving_Within_Cap (R : Fr_Bareme_2025_Pkg.Euros; Couple : Boolean; C : Children; Alone : Boolean) return Boolean is
    (Base_Tax (R, Couple) - Tax_Half_Cents (R, Couple, C, Alone) <= Cap_Allowance (Couple, C, Alone))
    with Post => Saving_Within_Cap'Result;

  function Never_Above_Base_Tax (R : Fr_Bareme_2025_Pkg.Euros; Couple : Boolean; C : Children; Alone : Boolean) return Boolean is
    (Tax_Half_Cents (R, Couple, C, Alone) <= Base_Tax (R, Couple))
    with Post => Never_Above_Base_Tax'Result;

  function Never_Negative (R : Fr_Bareme_2025_Pkg.Euros; Couple : Boolean; C : Children; Alone : Boolean) return Boolean is
    (Tax_Half_Cents (R, Couple, C, Alone) >= 0)
    with Post => Never_Negative'Result;

  function Parts_Counted (C : Children) return Boolean is
    ((Total_Half_Parts (True, C, False) >= 4) and then (Total_Half_Parts (False, C, True) >= 2))
    with Post => Parts_Counted'Result;

  function Official_Examples return Boolean is
    ((Tax_Half_Cents (130_000, True, 4, False) = 2_916_992) and then (Tax_Half_Cents (54_000, True, 2, False) = 429_198) and then (Tax_Half_Cents (80_000, False, 1, True) = 2_588_296) and then (Tax_Half_Cents (40_000, False, 1, True) = 374_132))
    with Post => Official_Examples'Result;

end Fr_Quotient_2025_Pkg;
