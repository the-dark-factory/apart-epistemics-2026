package Fr_Recouvrement_Pkg with SPARK_Mode, Pure is
   -- PURPOSE: French collection threshold: income tax below 61 euros is not collected.
   -- USAGE: call Collected (T) with T the tax in euros 0 .. 10**7.

   type Euros is range 0 .. 10_000_000;

   function Collected (T : Euros) return Euros is
     (if T < 61 then 0 else T);

   function Below_Threshold_Nothing (T : Euros) return Boolean is
     ((if T < 61 then Collected (T) = 0))
     with Post => Below_Threshold_Nothing'Result;

   function At_Or_Above_Unchanged (T : Euros) return Boolean is
     ((if T >= 61 then Collected (T) = T))
     with Post => At_Or_Above_Unchanged'Result;

   function Never_More_Than_Tax (T : Euros) return Boolean is
     (Collected (T) <= T)
     with Post => Never_More_Than_Tax'Result;

   function Monotone (T1, T2 : Euros) return Boolean is
     ((if T1 <= T2 then Collected (T1) <= Collected (T2)))
     with Post => Monotone'Result;

   function Official_Examples return Boolean is
     (Collected (60) = 0 and then Collected (61) = 61 and then Collected (0) = 0 and then Collected (2_146) = 2_146)
     with Post => Official_Examples'Result;

end Fr_Recouvrement_Pkg;
