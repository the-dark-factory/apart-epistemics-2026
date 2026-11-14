with Fr_Abattement_2025_Pkg;
with Fr_Bareme_2025_Pkg;
with Fr_Quotient_2025_Pkg;
with Fr_Decote_2025_Pkg;
with Fr_Recouvrement_Pkg;
package Fr_Ir_2025_Pkg with SPARK_Mode, Pure is
  -- PURPOSE: French income tax 2025 (2024 income) for salaried households, from salaries to the tax collected, in euros.
  -- USAGE: call Net_Tax (S1, S2, Couple, C, Alone): salaries in euros, C children, Alone = single parent living alone.
  use type Fr_Abattement_2025_Pkg.Wide;
  use type Fr_Bareme_2025_Pkg.Wide;
  use type Fr_Decote_2025_Pkg.Wide;
  use type Fr_Decote_2025_Pkg.Euros;
  use type Fr_Recouvrement_Pkg.Euros;
  type Salary is range 0 .. 4_000_000;
  function Net_Total (S1 : in Salary; S2 : in Salary; Couple : in Boolean) return Fr_Abattement_2025_Pkg.Wide is
    (Fr_Abattement_2025_Pkg.Net_Salary (Fr_Abattement_2025_Pkg.Euros (S1)) + (if Couple then Fr_Abattement_2025_Pkg.Net_Salary (Fr_Abattement_2025_Pkg.Euros (S2)) else 0));
  function Taxable (S1 : in Salary; S2 : in Salary; Couple : in Boolean) return Fr_Bareme_2025_Pkg.Euros is
    (Fr_Bareme_2025_Pkg.Euros ((Net_Total (S1, S2, Couple) + 5) / 10));
  function Brut (S1 : in Salary; S2 : in Salary; Couple : in Boolean; C : in Fr_Quotient_2025_Pkg.Children; Alone : in Boolean) return Fr_Decote_2025_Pkg.Euros is
    (Fr_Decote_2025_Pkg.Euros ((Fr_Quotient_2025_Pkg.Tax_Half_Cents (Taxable (S1, S2, Couple), Couple, C, Alone) + 100) / 200));
  function After (S1 : in Salary; S2 : in Salary; Couple : in Boolean; C : in Fr_Quotient_2025_Pkg.Children; Alone : in Boolean) return Fr_Decote_2025_Pkg.Wide is
    (Fr_Decote_2025_Pkg.After_Decote (Brut (S1, S2, Couple, C, Alone), Couple));
  function Net_Tax (S1 : in Salary; S2 : in Salary; Couple : in Boolean; C : in Fr_Quotient_2025_Pkg.Children; Alone : in Boolean) return Fr_Recouvrement_Pkg.Euros is
    ((if Fr_Recouvrement_Pkg.Collected (Fr_Recouvrement_Pkg.Euros (After (S1, S2, Couple, C, Alone) / 10_000)) = 0 then 0 else Fr_Recouvrement_Pkg.Euros ((After (S1, S2, Couple, C, Alone) + 5_000) / 10_000)));
  function Net_Never_More_Than_Brut (S1 : in Salary; S2 : in Salary; Couple : in Boolean; C : in Fr_Quotient_2025_Pkg.Children; Alone : in Boolean) return Boolean is
    ((Fr_Decote_2025_Pkg.Wide (Net_Tax (S1, S2, Couple, C, Alone)) <= Fr_Decote_2025_Pkg.Wide (Brut (S1, S2, Couple, C, Alone))))
      with Post => Net_Never_More_Than_Brut'Result;
  function Single_Ignores_Second_Salary (S1 : in Salary; S2 : in Salary; C : in Fr_Quotient_2025_Pkg.Children; Alone : in Boolean) return Boolean is
    ((Net_Tax (S1, S2, False, C, Alone) = Net_Tax (S1, 0, False, C, Alone)))
      with Post => Single_Ignores_Second_Salary'Result;
  function Official_Examples return Boolean is
    ((Brut (60_000, 0, True, 2, False) = 2_146 and then Net_Tax (60_000, 0, True, 2, False) = 1_647 and then Brut (144_426, 0, True, 4, False) = 14_585 and then Net_Tax (144_426, 0, True, 4, False) = 14_585 and then Net_Tax (19_384, 0, False, 0, False) = 0 and then Net_Tax (19_386, 0, False, 0, False) = 62 and then Net_Tax (15_000, 0, False, 0, False) = 0));
end Fr_Ir_2025_Pkg;
