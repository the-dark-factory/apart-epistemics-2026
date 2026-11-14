with Ada.Command_Line;
with Ada.Text_IO;
with Fr_Quotient_2025_Pkg;
with Fr_Ir_2025_Pkg;

procedure Fr_Ir_Main is
   function Is_Digits (S : String) return Boolean is
   begin
      if S'Length < 1 then
         return False;
      end if;
      for I in S'First .. S'Last loop
         if S (I) < '0' or else S (I) > '9' then
            return False;
         end if;
      end loop;
      return True;
   end Is_Digits;

   function Value_Of (S : String) return Long_Long_Integer is
      V : Long_Long_Integer := 0;
   begin
      for I in S'First .. S'Last loop
         V := V * 10 + Long_Long_Integer (Character'Pos (S (I)) - Character'Pos ('0'));
      end loop;
      return V;
   end Value_Of;

   function Text_Of (V : Long_Long_Integer) return String is
   begin
      if V < 10 then
         return (1 => Character'Val (Character'Pos ('0') + Integer (V)));
      else
         return Text_Of (V / 10) & Character'Val (Character'Pos ('0') + Integer (V mod 10));
      end if;
   end Text_Of;

   function Limit_Of (N : Positive) return Long_Long_Integer is
   begin
      if N = 1 or else N = 2 then
         return 4_000_000;
      elsif N = 4 then
         return 10;
      else
         return 1;
      end if;
   end Limit_Of;

   function Reason_Of (N : Positive; S : String) return String is
   begin
      if not Is_Digits (S) then
         return "NOT_DIGITS";
      elsif S'Length > 9 then
         return "OUT_OF_RANGE";
      elsif Value_Of (S) > Limit_Of (N) then
         return "OUT_OF_RANGE";
      else
         return "";
      end if;
   end Reason_Of;

   function Num (N : Positive) return Long_Long_Integer is
   begin
      return Value_Of (Ada.Command_Line.Argument (N));
   end Num;

   function Flag (N : Positive) return Boolean is
   begin
      return Num (N) = 1;
   end Flag;

   procedure Say_Refuse (Word : String) is
   begin
      Ada.Text_IO.Put_Line ("REFUSE " & Word);
      Ada.Command_Line.Set_Exit_Status (Ada.Command_Line.Exit_Status (2));
   end Say_Refuse;
begin
   if Ada.Command_Line.Argument_Count /= 5 then
      Say_Refuse ("ARG_COUNT");
   else
      declare
         First_Bad : Natural := 0;
      begin
         for N in 1 .. 5 loop
            if First_Bad = 0 and then Reason_Of (N, Ada.Command_Line.Argument (N)) /= "" then
               First_Bad := N;
            end if;
         end loop;
         if First_Bad /= 0 then
            Say_Refuse (Reason_Of (First_Bad, Ada.Command_Line.Argument (First_Bad)));
         elsif Flag (3) and then Flag (5) then
            Say_Refuse ("ALONE_AND_COUPLE");
         else
            declare
               S1 : constant Fr_Ir_2025_Pkg.Salary := Fr_Ir_2025_Pkg.Salary (Num (1));
               S2 : constant Fr_Ir_2025_Pkg.Salary := Fr_Ir_2025_Pkg.Salary (Num (2));
               Couple : constant Boolean := Flag (3);
               Kids : constant Fr_Quotient_2025_Pkg.Children := Fr_Quotient_2025_Pkg.Children (Num (4));
               Alone : constant Boolean := Flag (5);
            begin
               Ada.Text_IO.Put_Line ("Taxable=" & Text_Of (Long_Long_Integer (Fr_Ir_2025_Pkg.Taxable (S1, S2, Couple))) & " Brut=" & Text_Of (Long_Long_Integer (Fr_Ir_2025_Pkg.Brut (S1, S2, Couple, Kids, Alone))) & " Net=" & Text_Of (Long_Long_Integer (Fr_Ir_2025_Pkg.Net_Tax (S1, S2, Couple, Kids, Alone))));
               Ada.Command_Line.Set_Exit_Status (Ada.Command_Line.Success);
            end;
         end if;
      end;
   end if;
exception
   when others =>
      Ada.Text_IO.Put_Line ("FAILED");
      Ada.Command_Line.Set_Exit_Status (Ada.Command_Line.Exit_Status (3));
end Fr_Ir_Main;
