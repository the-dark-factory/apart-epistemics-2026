package Fr_Abattement_2025_Pkg with SPARK_Mode, Pure is
  -- PURPOSE: French 10 percent deduction on one salary, 2024 income: min 504, max 14 426, never above the salary.
  -- USAGE: call Deduction (S) or Net_Salary (S) with S the salary in euros 0 .. 10**7; results in tenths of a euro.

  type Euros is range 0 .. 10_000_000;

  type Wide is range -1_000_000_000_000 .. 1_000_000_000_000;

  function Lower (A, B : Wide) return Wide is
    ((if A < B then A else B));

  function Higher (A, B : Wide) return Wide is
    ((if A > B then A else B));

  function Deduction (S : Euros) return Wide is
    (Lower (10 * Wide (S), Higher (5_040, Lower (144_260, Wide (S)))));

  function Net_Salary (S : Euros) return Wide is
    (10 * Wide (S) - Deduction (S));

  function Between_Zero_And_Salary (S : Euros) return Boolean is
    ((Deduction (S) >= 0 and then Deduction (S) <= 10 * Wide (S)))
      with Post => Between_Zero_And_Salary'Result;

  function Ten_Percent_Inside_Limits (S : Euros) return Boolean is
    (((if Wide (S) >= 5_040 and then Wide (S) <= 144_260 then Deduction (S) = Wide (S))))
      with Post => Ten_Percent_Inside_Limits'Result;

  function Minimum_Applies (S : Euros) return Boolean is
    (((if Wide (S) >= 504 and then Wide (S) < 5_040 then Deduction (S) = 5_040)))
      with Post => Minimum_Applies'Result;

  function Small_Salary_Fully_Deducted (S : Euros) return Boolean is
    (((if Wide (S) < 504 then Net_Salary (S) = 0)))
      with Post => Small_Salary_Fully_Deducted'Result;

  function Maximum_Applies (S : Euros) return Boolean is
    (((if Wide (S) > 144_260 then Deduction (S) = 144_260)))
      with Post => Maximum_Applies'Result;

  function Net_Monotone (S1, S2 : Euros) return Boolean is
    (((if S1 <= S2 then Net_Salary (S1) <= Net_Salary (S2))))
      with Post => Net_Monotone'Result;

  function Official_Examples return Boolean is
    ((Deduction (30_000) = 30_000 and then Net_Salary (30_000) = 270_000 and then Deduction (300) = 3_000 and then Net_Salary (300) = 0 and then Deduction (2_000) = 5_040 and then Deduction (200_000) = 144_260))
      with Post => Official_Examples'Result;

end Fr_Abattement_2025_Pkg;
