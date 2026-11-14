package Fr_Bareme_2025_Pkg with SPARK_Mode, Pure is
   -- PURPOSE: French income tax scale 2025 (2024 income), applied per part and multiplied by the parts, in half-cents.
   -- USAGE: call Bareme_Half_Cents (R, H) with R net taxable euros 0 .. 10**7 and H half-parts 2 .. 40.

   type Euros is range 0 .. 10_000_000;
   type Half_Parts is range 2 .. 40;
   type Wide is range -1_000_000_000_000 .. 1_000_000_000_000;

   function Clip (X : in Wide) return Wide is
     ((if X < 0 then 0 else X));

   function Lower (X, Y : in Wide) return Wide is
     ((if X < Y then X else Y));

   function Bareme_Half_Cents (R : in Euros; H : in Half_Parts) return Wide is
     ((11 * Clip (Lower (2 * Wide (R), Wide (H) * 29_315) - Wide (H) * 11_497)
       + 30 * Clip (Lower (2 * Wide (R), Wide (H) * 83_823) - Wide (H) * 29_315)
       + 41 * Clip (Lower (2 * Wide (R), Wide (H) * 180_294) - Wide (H) * 83_823)
       + 45 * Clip (2 * Wide (R) - Wide (H) * 180_294)));

   function Zero_Up_To_First_Band (R : in Euros; H : in Half_Parts) return Boolean is
     ((if 2 * Wide (R) <= Wide (H) * 11_497 then Bareme_Half_Cents (R, H) = 0))
     with Post => Zero_Up_To_First_Band'Result;

   function Never_Negative (R : in Euros; H : in Half_Parts) return Boolean is
     ((Bareme_Half_Cents (R, H) >= 0))
     with Post => Never_Negative'Result;

   function At_Most_45_Percent (R : in Euros; H : in Half_Parts) return Boolean is
     ((Bareme_Half_Cents (R, H) <= 90 * Wide (R)))
     with Post => At_Most_45_Percent'Result;

   function Monotone_In_Income (R1, R2 : in Euros; H : in Half_Parts) return Boolean is
     ((if R1 <= R2 then Bareme_Half_Cents (R1, H) <= Bareme_Half_Cents (R2, H)))
     with Post => Monotone_In_Income'Result;

   function More_Parts_Never_More_Tax (R : in Euros; H1, H2 : in Half_Parts) return Boolean is
     ((if H1 <= H2 then Bareme_Half_Cents (R, H2) <= Bareme_Half_Cents (R, H1)))
     with Post => More_Parts_Never_More_Tax'Result;

   function Official_Examples return Boolean is
     (((Bareme_Half_Cents (54_000, 6) = 429_198) and then (Bareme_Half_Cents (130_000, 10) = 1_595_330) and then (Bareme_Half_Cents (130_000, 4) = 5_066_192) and then (Bareme_Half_Cents (11_497, 2) = 0)))
     with Post => Official_Examples'Result;

end Fr_Bareme_2025_Pkg;
