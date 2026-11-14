package Fr_Decote_2025_Pkg with SPARK_Mode, Pure is
  -- PURPOSE: French decote 2025 (2024 income): the reduction of a small tax bill, single or couple, in 1/10000 euro.
  -- USAGE: call After_Decote (T, Couple) with T the tax before the decote in euros 0 .. 10**7.

  type Euros is range 0 .. 10_000_000;

  type Wide is range -1_000_000_000_000 .. 1_000_000_000_000;

  function Clip (X : Wide) return Wide is
    ((if X < 0 then 0 else X));

  function Decote (T : Euros; Couple : Boolean) return Wide is
    ((if Couple and then Wide (T) < 3_249 then Clip (14_700_000 - 4_525 * Wide (T)) elsif not Couple and then Wide (T) < 1_965 then Clip (8_890_000 - 4_525 * Wide (T)) else 0));

  function After_Decote (T : Euros; Couple : Boolean) return Wide is
    (Clip (10_000 * Wide (T) - Decote (T, Couple)));

  function Between_Zero_And_Tax (T : Euros; Couple : Boolean) return Boolean is
    ((After_Decote (T, Couple) >= 0 and then After_Decote (T, Couple) <= 10_000 * Wide (T)))
      with Post => Between_Zero_And_Tax'Result;

  function No_Decote_At_Or_Above_Threshold (T : Euros; Couple : Boolean) return Boolean is
    ((if (Couple and then Wide (T) >= 3_249) or else (not Couple and then Wide (T) >= 1_965) then After_Decote (T, Couple) = 10_000 * Wide (T)))
      with Post => No_Decote_At_Or_Above_Threshold'Result;

  function Monotone_In_Tax (T1 : Euros; T2 : Euros; Couple : Boolean) return Boolean is
    ((if T1 <= T2 then After_Decote (T1, Couple) <= After_Decote (T2, Couple)))
      with Post => Monotone_In_Tax'Result;

  function Couple_Never_Pays_More (T : Euros) return Boolean is
    ((After_Decote (T, True) <= After_Decote (T, False)))
      with Post => Couple_Never_Pays_More'Result;

  function Official_Examples return Boolean is
    ((Decote (2_140, True) = 5_016_500 and then After_Decote (2_140, True) = 16_383_500 and then After_Decote (500, False) = 0 and then After_Decote (1_965, False) = 19_650_000 and then After_Decote (3_249, True) = 32_490_000))
      with Post => Official_Examples'Result;

end Fr_Decote_2025_Pkg;
