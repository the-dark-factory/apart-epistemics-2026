with Ada.Command_Line;
with Ada.Text_IO;
with Fact_Gate_Pkg;

procedure Fact_Gate_Main is
   function Is_Bit (S : String) return Boolean is
   begin
      if S'Length /= 1 then
         return False;
      end if;
      return S (S'First) = '0' or else S (S'First) = '1';
   end Is_Bit;

   function Text_Of (V : Long_Long_Integer) return String is
   begin
      if V < 10 then
         return (1 => Character'Val (Character'Pos ('0') + Integer (V)));
      else
         return Text_Of (V / 10) & Character'Val (Character'Pos ('0') + Integer (V mod 10));
      end if;
   end Text_Of;

   function Flag (N : Positive) return Boolean is
   begin
      return Ada.Command_Line.Argument (N) = "1";
   end Flag;

   procedure Say_Refuse (Word : String) is
   begin
      Ada.Text_IO.Put_Line ("REFUSE " & Word);
      Ada.Command_Line.Set_Exit_Status (Ada.Command_Line.Exit_Status (2));
   end Say_Refuse;

begin
   if Ada.Command_Line.Argument_Count /= 8 then
      Say_Refuse ("ARG_COUNT");
   else
      declare
         First_Bad : Natural := 0;
      begin
         for N in 1 .. 8 loop
            if First_Bad = 0 and then not Is_Bit (Ada.Command_Line.Argument (N)) then
               First_Bad := N;
            end if;
         end loop;
         if First_Bad /= 0 then
            Say_Refuse ("NOT_BIT");
         else
            declare
               P1 : constant Boolean := Flag (1);
               P2 : constant Boolean := Flag (2);
               P3 : constant Boolean := Flag (3);
               P4 : constant Boolean := Flag (4);
               P5 : constant Boolean := Flag (5);
               P6 : constant Boolean := Flag (6);
               P7 : constant Boolean := Flag (7);
               P8 : constant Boolean := Flag (8);
            begin
               if Fact_Gate_Pkg.Complete (P1, P2, P3, P4, P5, P6, P7, P8) then
                  Ada.Text_IO.Put_Line ("COMPLETE");
               else
                  Ada.Text_IO.Put_Line ("MISSING " & Text_Of (Long_Long_Integer (Fact_Gate_Pkg.First_Missing (P1, P2, P3, P4, P5, P6, P7, P8))));
               end if;
               Ada.Command_Line.Set_Exit_Status (Ada.Command_Line.Success);
            end;
         end if;
      end;
   end if;
exception
   when others =>
      Ada.Text_IO.Put_Line ("FAILED");
      Ada.Command_Line.Set_Exit_Status (Ada.Command_Line.Exit_Status (3));
end Fact_Gate_Main;
